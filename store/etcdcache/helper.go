package etcdcache

import (
	"fmt"
	"reflect"

	"golang.org/x/exp/maps"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"xiaoshiai.cn/common/store"
)

// UnstructuredObjectTyper provides a runtime.ObjectTyper implementation for
// runtime.Unstructured object based on discovery information.
type UnstructuredObjectTyper struct{}

// ObjectKinds returns a slice of one element with the group,version,kind of the
// provided object, or an error if the object is not runtime.Unstructured or
// has no group,version,kind information. unversionedType will always be false
// because runtime.Unstructured object should always have group,version,kind
// information set.
func (d UnstructuredObjectTyper) ObjectKinds(obj runtime.Object) (gvks []schema.GroupVersionKind, unversionedType bool, err error) {
	if _, ok := obj.(runtime.Unstructured); ok {
		gvk := obj.GetObjectKind().GroupVersionKind()
		if len(gvk.Kind) == 0 {
			return nil, false, runtime.NewMissingKindErr("object has no kind field ")
		}
		if len(gvk.Version) == 0 {
			return nil, false, runtime.NewMissingVersionErr("object has no apiVersion field")
		}
		return []schema.GroupVersionKind{gvk}, false, nil
	}
	return nil, false, runtime.NewNotRegisteredErrForType("crdserverscheme.UnstructuredObjectTyper", reflect.TypeOf(obj))
}

// Recognizes returns true if the provided group,version,kind was in the
// discovery information.
func (d UnstructuredObjectTyper) Recognizes(gvk schema.GroupVersionKind) bool {
	return false
}

var _ runtime.ObjectTyper = &UnstructuredObjectTyper{}

type UnstructuredCreator struct{}

func (c UnstructuredCreator) New(kind schema.GroupVersionKind) (runtime.Object, error) {
	ret := &unstructured.Unstructured{}
	ret.SetGroupVersionKind(kind)
	return ret, nil
}

func GetAttrs(obj runtime.Object) (labels.Set, fields.Set, error) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return nil, nil, err
	}
	objectMetaFields := fields.Set{
		"metadata.name":      accessor.GetName(),
		"metadata.namespace": accessor.GetNamespace(),
	}
	us, ok := obj.(runtime.Unstructured)
	if !ok {
		return nil, nil, fmt.Errorf("unexpected error casting a custom resource to unstructured")
	}
	uc := us.UnstructuredContent()
	maps.Copy(objectMetaFields, FlatterMap(uc))
	return accessor.GetLabels(), objectMetaFields, nil
}

func FlatterMap(m map[string]any) map[string]string {
	ret := make(map[string]string)
	for k, v := range m {
		switch val := v.(type) {
		case map[string]any:
			for kk, vv := range FlatterMap(val) {
				ret[k+"."+kk] = vv
			}
		default:
			ret[k] = store.AnyToString(v)
		}
	}
	return ret
}
