package etcdcache

import (
	"reflect"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
