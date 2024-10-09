package mongo

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"golang.org/x/exp/maps"
	spec "k8s.io/kube-openapi/pkg/validation/spec"
	libreflect "xiaoshiai.cn/common/reflect"
	"xiaoshiai.cn/common/store"
)

type UnionFields []string

type ObjectDefination struct {
	Uniques   []UnionFields
	Indexes   []UnionFields
	ScopeKeys []string
	Schema    *spec.Schema
}

var GlobalObjectsScheme = NewObjectScheme()

func NewObjectScheme() *ObjectScheme {
	return &ObjectScheme{
		resourceMap:        map[string]objectsSchemeCache{},
		typeToResource:     map[reflect.Type]string{},
		listTypeToResource: map[reflect.Type]string{},
	}
}

type objectsSchemeCache struct {
	Defination ObjectDefination
	ObjectType reflect.Type
}

// ObjectScheme is a registry of objects and lists
// it is used to create new objects and lists by resource name
// or use for resources discovery
type ObjectScheme struct {
	resourceMap        map[string]objectsSchemeCache
	typeToResource     map[reflect.Type]string
	listTypeToResource map[reflect.Type]string
	lock               sync.RWMutex
}

func (s *ObjectScheme) Register(object store.Object, o ObjectDefination) error {
	if object == nil {
		return fmt.Errorf("object is nil")
	}
	resource, err := store.GetResource(object)
	if err != nil {
		return err
	}
	t := reflect.TypeOf(object)
	s.lock.Lock()
	defer s.lock.Unlock()
	if _, ok := s.resourceMap[resource]; ok {
		return fmt.Errorf("resource %s already registered", resource)
	}
	s.resourceMap[resource] = objectsSchemeCache{Defination: o, ObjectType: t}
	s.typeToResource[t] = resource
	return nil
}

func (s *ObjectScheme) New(resource string) (store.Object, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	val, ok := s.resourceMap[resource]
	if !ok {
		return nil, fmt.Errorf("resource %s not registered", resource)
	}
	t := val.ObjectType
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
		return reflect.New(t).Interface().(store.Object), nil
	}
	return reflect.New(t).Elem().Interface().(store.Object), nil
}

func (s *ObjectScheme) Registered() []string {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return maps.Keys(s.resourceMap)
}

func (s *ObjectScheme) GetDefination(resource string) (ObjectDefination, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	val, ok := s.resourceMap[resource]
	if !ok {
		return ObjectDefination{}, fmt.Errorf("resource %s not registered", resource)
	}
	return val.Defination, nil
}

func ObjectFields(o store.Object) ([]string, error) {
	t := reflect.TypeOf(o)
	fields := []string{}
	err := flattenTypeFields("", t, 1, func(name string) error {
		fields = append(fields, strings.TrimPrefix(name, "."))
		return nil
	})
	return fields, err
}

func flattenTypeFields(name string, t reflect.Type, maxDepth int, fieldFunc func(string) error) error {
	if maxDepth <= 0 {
		return fieldFunc(name)
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return fieldFunc(name)
	}
	for i := 0; i < t.NumField(); i++ {
		structField := t.Field(i)
		isEmbedded, isIgnore, fieldName := libreflect.StructFieldInfo(structField)
		if isIgnore {
			continue
		}
		if isEmbedded {
			if err := flattenTypeFields(name, structField.Type, maxDepth, fieldFunc); err != nil {
				return err
			}
			continue
		}
		if err := flattenTypeFields(name+"."+fieldName, structField.Type, maxDepth-1, fieldFunc); err != nil {
			return err
		}
	}
	return nil
}
