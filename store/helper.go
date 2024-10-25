package store

import (
	"context"
	stderrors "errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"xiaoshiai.cn/common/errors"
)

type ObjectReference struct {
	Name   string  `json:"name,omitempty"`
	Scopes []Scope `json:"scopes,omitempty"`
}

func ObjectReferenceFrom(obj Object) ObjectReference {
	return ObjectReference{
		Name:   obj.GetName(),
		Scopes: obj.GetScopes(),
	}
}

func (r ObjectReference) String() string {
	key := ""
	for _, scope := range r.Scopes {
		key += "/" + scope.Resource + "/" + scope.Name
	}
	key += "/" + r.Name
	return key
}

type ResourcedObjectReference struct {
	Name     string  `json:"name,omitempty"`
	Scopes   []Scope `json:"scopes,omitempty"`
	Resource string  `json:"resource,omitempty"`
}

func (r ResourcedObjectReference) String() string {
	key := ""
	for _, scope := range r.Scopes {
		key += "/" + scope.Resource + "/" + scope.Name
	}
	key += "/" + r.Resource
	key += "/" + r.Name
	return key
}

func ResourcedObjectReferenceFrom(obj Object) ResourcedObjectReference {
	return ResourcedObjectReference{
		Name:     obj.GetName(),
		Scopes:   obj.GetScopes(),
		Resource: obj.GetResource(),
	}
}

func ObjectIdentity(obj Object) string {
	key := ""
	for _, scope := range obj.GetScopes() {
		key += "/" + scope.Resource + "/" + scope.Name
	}
	key += "/" + obj.GetResource()
	key += "/" + obj.GetName()
	return key
}

func ScopesEquals(a, b []Scope) bool {
	if len(a) != len(b) {
		return false
	}
	for i, scope := range a {
		if scope != b[i] {
			return false
		}
	}
	return true
}

func GrowSlice(v reflect.Value, maxCapacity int, sizes ...int) {
	cap := v.Cap()
	max := cap
	for _, size := range sizes {
		if size > max {
			max = size
		}
	}
	if len(sizes) == 0 || max > maxCapacity {
		max = maxCapacity
	}
	if max <= cap {
		return
	}
	if v.Len() > 0 {
		extra := reflect.MakeSlice(v.Type(), v.Len(), max)
		reflect.Copy(extra, v)
		v.Set(extra)
	} else {
		extra := reflect.MakeSlice(v.Type(), 0, max)
		v.Set(extra)
	}
}

func SetFinalizer(obj Object, finalizer string) Object {
	finalizers := obj.GetFinalizers()
	if slices.Contains(finalizers, finalizer) {
		return obj
	}
	obj.SetFinalizers(append(finalizers, finalizer))
	return obj
}

func RemoveFinalizer(o Object, finalizer string) (finalizersUpdated bool) {
	f := o.GetFinalizers()
	length := len(f)

	index := 0
	for i := 0; i < length; i++ {
		if f[i] == finalizer {
			continue
		}
		f[index] = f[i]
		index++
	}
	o.SetFinalizers(f[:index])
	return length != index
}

func ContainsFinalizer(o Object, finalizer string) bool {
	f := o.GetFinalizers()
	for _, e := range f {
		if e == finalizer {
			return true
		}
	}
	return false
}

func IgnoreNotFound(err error) error {
	if errors.IsNotFound(err) {
		return nil
	}
	return err
}

func IgnoreAlreadyExists(err error) error {
	if errors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

var objcType = reflect.TypeOf((*Object)(nil)).Elem()

// NewItemFuncFromList returns the reflect.Value of the Items field of the list and a function to create a new item.
func NewItemFuncFromList(list ObjectList) (reflect.Value, func() Object, error) {
	listPtr, err := GetItemsPtr(list)
	if err != nil {
		return reflect.Value{}, nil, errors.NewBadRequest(fmt.Sprintf("object list must have Items field: %v", err))
	}
	v, err := EnforcePtr(listPtr)
	if err != nil || v.Kind() != reflect.Slice {
		return reflect.Value{}, nil, errors.NewBadRequest(fmt.Sprintf("object Items field must be a slice: %v", err))
	}
	elem := v.Type().Elem()
	if elem.Kind() == reflect.Ptr {
		return reflect.Value{}, nil, errors.NewBadRequest("object Items field must be a slice of non-pointer")
	}
	if !reflect.PointerTo(elem).Implements(objcType) {
		return reflect.Value{}, nil, errors.NewBadRequest("object Items field must be a slice of store.Object type")
	}
	newItemFunc := func() Object {
		return reflect.New(elem).Interface().(Object)
	}
	return v, newItemFunc, nil
}

func GetItemsPtr(list any) (any, error) {
	obj, err := getItemsPtr(list)
	if err != nil {
		return nil, fmt.Errorf("%T is not a list: %v", list, err)
	}
	return obj, nil
}

// getItemsPtr returns a pointer to the list object's Items member or an error.
func getItemsPtr(list any) (any, error) {
	v, err := EnforcePtr(list)
	if err != nil {
		return nil, err
	}

	items := v.FieldByName("Items")
	if !items.IsValid() {
		return nil, stderrors.New("Items field not found")
	}
	switch items.Kind() {
	case reflect.Interface, reflect.Pointer:
		target := reflect.TypeOf(items.Interface()).Elem()
		if target.Kind() != reflect.Slice {
			return nil, stderrors.New("Items field is not a slice")
		}
		return items.Interface(), nil
	case reflect.Slice:
		return items.Addr().Interface(), nil
	default:
		return nil, stderrors.New("Items field is not a slice")
	}
}

func CreateOrUpdate(ctx context.Context, store Store, obj Object, updatefn func() error) error {
	if err := store.Get(ctx, obj.GetName(), obj); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		if err := updatefn(); err != nil {
			return err
		}
		if err := store.Create(ctx, obj); err != nil {
			return err
		}
		return nil
	}
	if err := updatefn(); err != nil {
		return err
	}
	return store.Update(ctx, obj)
}

func EnforcePtr(obj any) (reflect.Value, error) {
	v := reflect.ValueOf(obj)
	if v.Kind() != reflect.Pointer {
		if v.Kind() == reflect.Invalid {
			return reflect.Value{}, fmt.Errorf("expected pointer, but got invalid kind")
		}
		return reflect.Value{}, fmt.Errorf("expected pointer, but got %v type", v.Type())
	}
	if v.IsNil() {
		return reflect.Value{}, fmt.Errorf("expected pointer, but got nil")
	}
	return v.Elem(), nil
}

func ForEachItem(list ObjectList, fn func(Object) error) error {
	items, err := GetItemsPtr(list)
	if err != nil {
		return err
	}
	v := reflect.ValueOf(items)
	v = reflect.Indirect(v)
	for i := 0; i < v.Len(); i++ {
		itemv := v.Index(i)
		// if item is not a pointer, we need to get its address
		if itemv.Kind() != reflect.Ptr {
			itemv = itemv.Addr()
		}
		item, ok := itemv.Interface().(Object)
		if !ok {
			return fmt.Errorf("item is not a Object: %T", itemv.Interface())
		}
		if err := fn(item); err != nil {
			return err
		}
	}
	return nil
}

func CreateIfNotExists(ctx context.Context, store Store, obj Object) error {
	if err := store.Create(ctx, obj); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
	}
	return nil
}

func SearchNameFunc[T any](search string, getname func(T) string) func(T) bool {
	if getname == nil || search == "" {
		return nil
	}
	return func(item T) bool {
		return strings.Contains(getname(item), search)
	}
}

func SortByFunc[T any](by string, getname func(T) string, gettime func(T) time.Time) func(a, b T) int {
	switch by {
	case "createTime", "createTimeAsc", "time":
		if gettime == nil {
			return nil
		}
		return func(a, b T) int {
			if timcmp := gettime(a).Compare(gettime(b)); timcmp == 0 && getname != nil {
				return strings.Compare(getname(a), getname(b))
			} else {
				return timcmp
			}
		}
	case "createTimeDesc", "time-", "": // default sort by time desc
		if gettime == nil {
			return nil
		}
		return func(a, b T) int {
			if timcmp := gettime(b).Compare(gettime(a)); timcmp == 0 && getname != nil {
				return strings.Compare(getname(a), getname(b))
			} else {
				return timcmp
			}
		}
	case "name":
		if getname == nil {
			return nil
		}
		return func(a, b T) int {
			return strings.Compare(getname(a), getname(b))
		}
	case "nameDesc", "name-":
		if getname == nil {
			return nil
		}
		return func(a, b T) int {
			return strings.Compare(getname(b), getname(a))
		}
	default:
		return nil
	}
}

func NewTimeAsName() string {
	return time.Now().Format("20060102150405")
}
