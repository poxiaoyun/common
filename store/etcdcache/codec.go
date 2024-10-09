package etcdcache

import (
	stdjson "encoding/json"
	"io"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/types"
)

var _ runtime.Codec = &SimpleJsonCodec{}

type SimpleJsonCodec struct{}

// Decode implements runtime.Codec.
func (s SimpleJsonCodec) Decode(data []byte, defaults *schema.GroupVersionKind, into runtime.Object) (runtime.Object, *schema.GroupVersionKind, error) {
	uns, ok := into.(*unstructured.Unstructured)
	if ok {
		if err := stdjson.Unmarshal(data, &uns.Object); err != nil {
			return nil, nil, err
		}
		// backcompat for old serialized objects
		resource, _, _ := unstructured.NestedString(uns.Object, "resource")
		if resource != "" {
			uns.SetKind(resource)
		}
		uid, _, _ := unstructured.NestedString(uns.Object, "uid")
		if uid != "" {
			uns.SetUID(types.UID(uid))
		}
		name, _, _ := unstructured.NestedString(uns.Object, "name")
		if name != "" {
			uns.SetName(name)
		}
		finalizers, _, _ := unstructured.NestedStringSlice(uns.Object, "finalizers")
		if len(finalizers) > 0 {
			uns.SetFinalizers(finalizers)
		}

		return uns, defaults, nil
	}
	if err := stdjson.Unmarshal(data, into); err != nil {
		return nil, nil, err
	}
	return into, defaults, nil
}

// Encode implements runtime.Codec.
func (s SimpleJsonCodec) Encode(obj runtime.Object, w io.Writer) error {
	return stdjson.NewEncoder(w).Encode(obj)
}

// Identifier implements runtime.Codec.
func (s SimpleJsonCodec) Identifier() runtime.Identifier {
	return runtime.Identifier("simplejson")
}

var _ json.MetaFactory = &DelegatedMetaFactory{}

type DelegatedMetaFactory struct {
	Delegate json.MetaFactory
}

var DefaultMetaFactory = &DelegatedMetaFactory{
	Delegate: json.DefaultMetaFactory,
}

// Interpret implements json.MetaFactory.
func (d *DelegatedMetaFactory) Interpret(data []byte) (*schema.GroupVersionKind, error) {
	findKind := struct {
		// +optional
		APIVersion string `json:"apiVersion,omitempty"`
		// +optional
		Resource string `json:"resource,omitempty"`
	}{}
	if err := stdjson.Unmarshal(data, &findKind); err != nil {
		return nil, err
	}
	if findKind.Resource != "" {
		gv, err := schema.ParseGroupVersion(findKind.APIVersion)
		if err != nil {
			return nil, err
		}
		return &schema.GroupVersionKind{Group: gv.Group, Version: gv.Version, Kind: findKind.Resource}, nil
	}
	return d.Delegate.Interpret(data)
}
