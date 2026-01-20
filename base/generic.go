package base

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/labels"
	"xiaoshiai.cn/common/controller"
	"xiaoshiai.cn/common/rest/api"
	"xiaoshiai.cn/common/store"
)

func GenericListWithWatch[T store.ObjectList](w http.ResponseWriter, r *http.Request, storage store.Store, list T, options ...store.ListOption) (any, error) {
	if api.Query(r, "watch", false) {
		return nil, GenericWatch(w, r, storage, list, options...)
	}
	return GenericList(r, storage, list, options...)
}

func GenericList[T store.ObjectList](r *http.Request, storage store.Store, list T, options ...store.ListOption) (T, error) {
	if err := GenericListFromRequest(r, storage, list, options...); err != nil {
		return *new(T), err
	}
	return list, nil
}

func GenericListFromRequest(r *http.Request, storage store.Store, list store.ObjectList, options ...store.ListOption) error {
	reqOptions, err := ListOptionsToStoreListOptions(api.GetListOptions(r))
	if err != nil {
		return err
	}
	options = append(reqOptions, options...)
	if err := storage.List(r.Context(), list, options...); err != nil {
		return err
	}
	return nil
}

func GenericWatch[T store.ObjectList](w http.ResponseWriter, r *http.Request, storage store.Store, list T, options ...store.ListOption) error {
	return GenericWatchWithName(w, r, storage, list, "", options...)
}

func GenericWatchWithName[T store.ObjectList](w http.ResponseWriter, r *http.Request, storage store.Store, list T, id string, options ...store.ListOption) error {
	listoptions := &store.ListOptions{}
	for _, option := range options {
		option(listoptions)
	}
	watchoption := func(o *store.WatchOptions) {
		o.LabelRequirements = listoptions.LabelRequirements
		o.FieldRequirements = listoptions.FieldRequirements
		o.ResourceVersion = listoptions.ResourceVersion
		o.ID = id
	}

	resource, err := store.GetResource(list)
	if err != nil {
		return err
	}

	encoder, err := api.NewStreamEncoderFromRequest[any](w, r)
	if err != nil {
		return err
	}
	handler := controller.EventHandlerFunc[*store.Unstructured](
		func(ctx context.Context, kind store.WatchEventType, obj *store.Unstructured) error {
			return encoder.Encode(string(kind), obj)
		},
	)
	return controller.RunWatch(r.Context(), storage, resource, handler, watchoption)
}

func GenericGet(r *http.Request, storage store.Store, obj store.Object, id string, options ...store.GetOption) (any, error) {
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	if err := storage.Get(r.Context(), id, obj, options...); err != nil {
		return nil, err
	}
	return obj, nil
}

func GenericCreate(r *http.Request, storage store.Store, obj store.Object, options ...store.CreateOption) (any, error) {
	if err := api.Body(r, obj); err != nil {
		return nil, err
	}
	if err := storage.Create(r.Context(), obj, options...); err != nil {
		return nil, err
	}
	return obj, nil
}

func GenericPatch(r *http.Request, storage store.Store, obj store.Object, id string, options ...store.PatchOption) (any, error) {
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	if objid := obj.GetID(); objid != "" && objid != id {
		return nil, fmt.Errorf("id in body %s is not equal to id in path %s", objid, id)
	}
	obj.SetID(id)
	patchmap := map[string]any{}
	if err := api.Body(r, &patchmap); err != nil {
		return nil, err
	}
	patch := MergePatchFromStruct(patchmap)
	if err := storage.Patch(r.Context(), obj, patch, options...); err != nil {
		return nil, err
	}
	return obj, nil
}

func MergePatchFromStruct(data any) store.Patch {
	b, _ := json.Marshal(data)
	return store.RawPatch(store.PatchTypeMergePatch, b)
}

func GenericUpdate(r *http.Request, storage store.Store, obj store.Object, id string, options ...store.UpdateOption) (any, error) {
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	if err := api.Body(r, obj); err != nil {
		return nil, err
	}
	if objid := obj.GetID(); objid != "" && objid != id {
		return nil, fmt.Errorf("id in body %s is not equal to id in path %s", objid, id)
	}
	obj.SetID(id)
	obj.SetResourceVersion(0)
	if err := storage.Update(r.Context(), obj, options...); err != nil {
		return nil, err
	}
	return obj, nil
}

func GenericDelete(r *http.Request, storage store.Store, obj store.Object, id string, options ...store.DeleteOption) (any, error) {
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	if objid := obj.GetID(); objid != "" && objid != id {
		return nil, fmt.Errorf("id in body %s is not equal to id in path %s", objid, id)
	}
	obj.SetID(id)
	if err := storage.Delete(r.Context(), obj, options...); err != nil {
		return nil, err
	}
	return obj, nil
}

func ParseLabelSelector(selectString string) (labels.Selector, error) {
	if selectString == "" {
		return nil, nil
	}
	return labels.Parse(selectString)
}
