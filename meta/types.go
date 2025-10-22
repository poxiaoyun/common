package meta

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Page[T any] struct {
	Total int64 `json:"total"`
	Items []T   `json:"items"`
	Page  int64 `json:"page"`
	Size  int64 `json:"size"`

	// for pagination
	// if continue is not empty, means there are more items
	// when use continue style pagination, total is not returned
	// and page is ignored
	Continue string `json:"continue,omitempty"`
}

// +k8s:openapi-gen=true
type Time = metav1.Time

type Duration = metav1.Duration

func Now() Time {
	return Time(metav1.Now())
}

// Empty represents an empty struct
// useful for as a map value when you only care about the keys
// it better than using struct{} directly, because it is more readable
// exmple:
//
//	myMap := map[string]meta.Empty{}
//	instead of
//	myMap := map[string]struct{}{}
type Empty struct{}

// Or returns the first non-zero value from the given values
// example:
//
//	val := Or(a, b, c)
func Or[T comparable](vals ...T) T {
	var zero T
	for _, v := range vals {
		if v != zero {
			return v
		}
	}
	return zero
}

// Tenary returns v1 if cond is true, otherwise returns v2
func Tenary[T any](cond bool, v1, v2 T) T {
	if cond {
		return v1
	}
	return v2
}

// DerefPtr dereferences a pointer, if the pointer is nil, returns the default value
func DerefPtr[T any](ptr *T, defaultVal T) T {
	if ptr == nil {
		return defaultVal
	}
	return *ptr
}

// Ptr returns a pointer to the given value
func Ptr[T any](v T) *T {
	return &v
}
