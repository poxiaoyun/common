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
