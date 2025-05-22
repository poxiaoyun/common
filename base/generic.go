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
	labelsSelector, err := ParseLabelSelector(api.Query(r, "label-selector", ""))
	if err != nil {
		return err
	}
	fieldsSelector, err := store.ParseRequirements(api.Query(r, "field-selector", ""))
	if err != nil {
		return err
	}
	reqlistopetions := api.GetListOptions(r)
	reqOptions := []store.ListOption{
		store.WithPageSize(reqlistopetions.Page, reqlistopetions.Size),
		store.WithSort(reqlistopetions.Sort),
		store.WithSearch(reqlistopetions.Search),
	}
	if labelsSelector != nil {
		reqOptions = append(reqOptions, store.WithLabelRequirementsFromSelector(labelsSelector))
	}
	if fieldsSelector != nil {
		reqOptions = append(reqOptions, store.WithFieldRequirements(fieldsSelector...))
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

func GenericWatchWithName[T store.ObjectList](w http.ResponseWriter, r *http.Request, storage store.Store, list T, name string, options ...store.ListOption) error {
	listoptions := &store.ListOptions{}
	for _, option := range options {
		option(listoptions)
	}
	watchoption := func(o *store.WatchOptions) {
		o.LabelRequirements = listoptions.LabelRequirements
		o.FieldRequirements = listoptions.FieldRequirements
		o.ResourceVersion = listoptions.ResourceVersion
		o.Name = name
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

func GenericGet(r *http.Request, storage store.Store, obj store.Object, name string, options ...store.GetOption) (any, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if err := storage.Get(r.Context(), name, obj, options...); err != nil {
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

func GenericPatch(r *http.Request, storage store.Store, obj store.Object, name string, options ...store.PatchOption) (any, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if objname := obj.GetName(); objname != "" && objname != name {
		return nil, fmt.Errorf("name in body %s is not equal to name in path %s", objname, name)
	}
	obj.SetName(name)
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

func GenericUpdate(r *http.Request, storage store.Store, obj store.Object, name string, options ...store.UpdateOption) (any, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if err := api.Body(r, obj); err != nil {
		return nil, err
	}
	if objname := obj.GetName(); objname != "" && objname != name {
		return nil, fmt.Errorf("name in body %s is not equal to name in path %s", objname, name)
	}
	obj.SetName(name)
	obj.SetResourceVersion(0)
	if err := storage.Update(r.Context(), obj, options...); err != nil {
		return nil, err
	}
	return obj, nil
}

func GenericDelete(r *http.Request, storage store.Store, obj store.Object, name string, options ...store.DeleteOption) (any, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if objname := obj.GetName(); objname != "" && objname != name {
		return nil, fmt.Errorf("name in body %s is not equal to name in path %s", objname, name)
	}
	obj.SetName(name)
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
