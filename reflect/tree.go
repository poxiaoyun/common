package reflect

import (
	"reflect"
	"strconv"
	"strings"
)

type Node struct {
	Name   string
	Kind   reflect.Kind
	Tag    reflect.StructTag
	Value  reflect.Value
	Fields []Node
}

func ParseStruct(data any) Node {
	return decode("", "", reflect.ValueOf(data))
}

func ToJsonPathes(prefix string, nodes []Node) []KV {
	return toJsonPathes(prefix, nodes, []KV{})
}

func prefixedKey(prefix, key string, splitor ...string) string {
	if len(prefix) == 0 {
		return strings.ToLower(key)
	}

	spl := "-"
	if len(splitor) > 0 {
		spl = string(splitor[0])
	}
	return strings.ToLower(prefix + spl + key)
}

type KV struct {
	Key   string
	Value any
}

func toJsonPathes(prefix string, nodes []Node, kvs []KV) []KV {
	for _, node := range nodes {
		switch node.Kind {
		case reflect.Struct, reflect.Map:
			kvs = toJsonPathes(prefixedKey(prefix, node.Name, "."), node.Fields, kvs)
		default:
			kvs = append(kvs, KV{
				Key:   prefixedKey(prefix, node.Name, "."),
				Value: node.Value.Interface(),
			})
		}
	}
	return kvs
}

func decode(name string, tag reflect.StructTag, v reflect.Value) Node {
	var fields []Node
	switch v.Kind() {
	case reflect.Struct:
		if name == "" {
			name = v.Type().Name()
		}
		for i := 0; i < v.NumField(); i++ {
			fi, fv := v.Type().Field(i), v.Field(i)
			isEmbedded, isIgnored, fieldName := StructFieldInfo(fi)
			if isIgnored {
				continue
			}
			fieldNode := decode(fieldName, fi.Tag, fv)
			if isEmbedded {
				fields = append(fields, fieldNode.Fields...)
			} else {
				fields = append(fields, fieldNode)
			}
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			fields = append(fields, decode(k.String(), "", v.MapIndex(k)))
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			fields = append(fields, decode(strconv.Itoa(i), "", v.Index(i)))
		}
	case reflect.Pointer:
		if !v.IsNil() {
			return decode(name, tag, v.Elem())
		}
	}
	return Node{
		Name:   name,
		Kind:   v.Kind(),
		Tag:    tag,
		Value:  v,
		Fields: fields,
	}
}
