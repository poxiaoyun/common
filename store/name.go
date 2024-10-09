package store

import (
	"fmt"
	"reflect"
	"strings"
)

func GetResource(obj any) (string, error) {
	t := reflect.TypeOf(obj)
	switch val := obj.(type) {
	case Object:
		if resource := val.GetResource(); resource != "" {
			return resource, nil
		}
	case ObjectList:
		if resource := val.GetResource(); resource != "" {
			return resource, nil
		}
		itemsPointer, err := GetItemsPtr(val)
		if err != nil {
			return "", err
		}
		t = reflect.TypeOf(itemsPointer).Elem().Elem()
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return ReflectResourceName(t)
}

func ReflectResourceName(t reflect.Type) (string, error) {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	typename := t.Name()
	// remove generic type suffix
	if i := strings.Index(typename, "["); i != -1 {
		typename = typename[:i]
	}
	if typename == "" {
		return "", fmt.Errorf("cannot get resource name from type %v", t)
	}
	return SimpleNameToPlural(strings.ToLower(typename)), nil
}

func SimpleNameToPlural(name string) string {
	if strings.HasSuffix(name, "s") {
		return name
	}
	if strings.HasSuffix(name, "y") {
		return strings.TrimSuffix(name, "y") + "ies"
	}
	return strings.ToLower(name) + "s"
}
