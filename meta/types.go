package meta

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Empty represents an empty struct
// useful for as a map value when you only care about the keys
// it better than using struct{} directly, because it is more readable
// exmple:
//
//	myMap := map[string]meta.Empty{}
//	instead of
//	myMap := map[string]struct{}{}
type Empty struct{}

type Page[T any] struct {
	// Total is the total number of items matching the query
	// it used for page style pagination
	Total int64 `json:"total"`
	// Items is the list of items in the current page
	Items []T `json:"items"`
	// Page is the current page number
	Page int64 `json:"page"`
	// Size is the number of items per page
	Size int64 `json:"size"`
	// for continue style pagination
	// if continue is not empty, means there are more items
	// when use continue style pagination, total is not returned and page is ignored
	// for next page, use the continue token to get next page
	Continue string `json:"continue,omitempty"`
}

type ListOptions struct {
	// Size is the number of items per page
	Size int `json:"size,omitempty"`
	// Page is the page number, starting from 1
	// it used for page style pagination
	Page int `json:"page,omitempty"`
	// Search is the search string
	// example:
	// search="test" will match objects with name or description contains "test"
	// search="name:test" will match objects with name contains "test"
	// search="name:test,description:demo" will match objects with name contains "test" or description contains "demo"
	// see [ParseSearch]
	Search string `json:"search,omitempty"`
	// Sort is the sort order of the list.  The format is a comma separated list of fields, optionally
	// prefixed by "+" or "-".  The default is "+metadata.name", which sorts by the object's name.
	// For example, "-metadata.name,metadata.creationTimestamp" sorts first by descending name, and then by
	// ascending creation timestamp.
	// name is alias for metadata.name
	// time is alias for metadata.creationTimestamp
	// see [ParseSort]
	Sort string `json:"sort,omitempty"`
	// Continue is a token to continue the list
	// it is used for continue style pagination
	Continue string `json:"continue,omitempty"`
	// FieldSelector is a selector expr to filter objects by fields
	// example: "metadata.name=myname,metadata.namespace=mynamespace"
	FieldSelector string `json:"fieldSelector,omitempty"`
	// LabelSelector is a selector expr to filter objects by labels
	// example: "app=myapp,env=prod"
	LabelSelector string `json:"labelSelector,omitempty"`
}

type SortDirection string

const (
	SortDirectionUnknown SortDirection = ""
	SortDirectionAsc     SortDirection = "asc"
	SortDirectionDesc    SortDirection = "desc"
)

type SortField struct {
	Field     string        `json:"field,omitempty"`
	Direction SortDirection `json:"direction,omitempty"`
}

// ParseSort parse a sort query string into a list of SortBy
// example: "name-,time+" => []SortBy{{Field: "name", ASC: false}, {Field: "time", ASC: true}}
func ParseSort(sort string) []SortField {
	if sort == "" {
		return nil
	}
	sortbys := []SortField{}
	for s := range strings.SplitSeq(sort, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		var direction SortDirection
		switch s[len(s)-1] {
		case '-':
			direction = SortDirectionDesc
			s = s[:len(s)-1]
		case '+':
			direction = SortDirectionAsc
			s = s[:len(s)-1]
		}
		sortbys = append(sortbys, SortField{Field: s, Direction: direction})
	}
	return sortbys
}

type FieldValue struct {
	Field string `json:"field"`
	Value any    `json:"value"`
}

// ParseSearch parse a search string into a list of FieldValue
// example: "name:tom,description:developer" => []FieldValue{{Field: "name", Value: "tom"}, {Field: "description", Value: "developer"}}
// if no field is specified, use "name" as the default field
// example: "tom" => []FieldValue{{Field: "name", Value: "tom"}}
func ParseSearch(search string) []FieldValue {
	if search == "" {
		return nil
	}
	fvs := []FieldValue{}
	for part := range strings.SplitSeq(search, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idx := strings.Index(part, ":")
		if idx == -1 {
			fvs = append(fvs, FieldValue{Field: "name", Value: part})
		} else {
			field := strings.TrimSpace(part[:idx])
			value := strings.TrimSpace(part[idx+1:])
			if field != "" && value != "" {
				fvs = append(fvs, FieldValue{Field: field, Value: value})
			}
		}
	}
	return fvs
}

// +k8s:openapi-gen=true
type Time = metav1.Time

type Duration = metav1.Duration

func Now() Time {
	return Time(metav1.Now())
}
