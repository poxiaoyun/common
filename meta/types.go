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

// Page is a flat list response. Exactly one of page, continuation, or
// unpaginated metadata is present on a successful response.
type Page[T any] struct {
	// ResourceVersion identifies the collection snapshot when the backend can
	// provide one. It is distinct from each item's resourceVersion.
	ResourceVersion int64 `json:"resourceVersion,omitempty"`
	// Total is the exact number of matching items for page or unpaginated
	// results. Continuation results omit it.
	Total *int `json:"total,omitempty"`
	// Items is the list of items in the current page
	Items []T `json:"items"`
	// Page is the current one-based page number for page pagination.
	Page int `json:"page,omitempty"`
	// Size is the number of items per page for page pagination.
	Size int `json:"size,omitempty"`
	// Continue is the opaque next-batch token. On a continuation response,
	// an omitted or empty token means that iteration is complete.
	Continue string `json:"continue,omitempty"`
	// Limit is the maximum number of items returned by continuation pagination.
	Limit int `json:"limit,omitempty"`
}

// ConvertPage maps list items while preserving collection pagination metadata.
func ConvertPage[T any, R any](page Page[T], convert func(T) R) Page[R] {
	result := Page[R]{
		ResourceVersion: page.ResourceVersion,
		Total:           page.Total,
		Page:            page.Page,
		Size:            page.Size,
		Continue:        page.Continue,
		Limit:           page.Limit,
		Items:           make([]R, 0, len(page.Items)),
	}
	for _, item := range page.Items {
		result.Items = append(result.Items, convert(item))
	}
	return result
}

// ListOptions contains flat list filters and pagination values. A positive
// Limit selects continuation pagination; otherwise a positive Size selects
// page pagination, with Page values below one treated as one. When neither is
// positive, the owning service chooses its unpaginated behavior.
type ListOptions struct {
	// Page is the one-based page number for page pagination.
	Page int `json:"page,omitempty"`
	// Size is the number of items per page for page pagination.
	Size int `json:"size,omitempty"`
	// Search is the search string
	// example:
	// search="test" will match objects with name or description contains "test"
	// search="name:test" will match objects with name contains "test"
	// search="name:test,description:demo" will match objects with name contains "test" or description contains "demo"
	// see [ParseSearch]
	Search string `json:"search,omitempty"`
	// Sort is the sort order of the list. The format is a comma-separated list of fields, optionally
	// suffixed by "+" or "-". The default is "metadata.name+", which sorts by the object's name.
	// For example, "metadata.name-,metadata.creationTimestamp+" sorts first by descending name, and then by
	// ascending creation timestamp.
	// name is alias for metadata.name
	// time is alias for metadata.creationTimestamp
	// see [ParseSort]
	Sort string `json:"sort,omitempty"`
	// Continue is an optional opaque token for a later continuation batch.
	// An empty value is equivalent to omitting it.
	Continue string `json:"continue,omitempty"`
	// Limit is the maximum number of items returned by continuation pagination.
	Limit int `json:"limit,omitempty"`
	// FieldSelector is a selector expr to filter objects by fields
	// example: "metadata.name=myname,metadata.namespace=mynamespace"
	FieldSelector string `json:"fieldSelector,omitempty"`
	// LabelSelector is a selector expr to filter objects by labels
	// example: "app=myapp,env=prod"
	LabelSelector string `json:"labelSelector,omitempty"`
}

// ListOption applies caller-owned policy to public list options.
type ListOption interface {
	// ApplyToList applies this option to options in declaration order.
	ApplyToList(*ListOptions)
}

// DefaultPageOption supplies default page and size values.
type DefaultPageOption struct {
	// Page is the one-based default page number.
	Page int
	// Size is the default number of items per page.
	Size int
}

// ApplyToList fills Page and Size independently unless Limit already selects continuation pagination.
func (option DefaultPageOption) ApplyToList(options *ListOptions) {
	// A continuation token or positive Limit expresses continuation intent. Do
	// not let page defaults replace that explicit request.
	if options.Continue != "" || options.Limit > 0 {
		return
	}
	if options.Page == 0 {
		options.Page = option.Page
	}
	if options.Size == 0 {
		options.Size = option.Size
	}
}

// DefaultPage supplies default page and size values.
func DefaultPage(page, size int) DefaultPageOption {
	return DefaultPageOption{Page: page, Size: size}
}

// DefaultContinuationOption supplies a default continuation limit.
type DefaultContinuationOption int

// ApplyToList fills Limit when it is zero unless Size already selects page pagination.
func (option DefaultContinuationOption) ApplyToList(options *ListOptions) {
	// A page number or positive Size expresses page intent. Do not let a
	// continuation default replace that explicit request through Limit priority.
	if options.Limit == 0 && options.Page == 0 && options.Size <= 0 {
		options.Limit = int(option)
	}
}

// DefaultContinuation supplies a default continuation limit.
func DefaultContinuation(limit int) DefaultContinuationOption {
	return DefaultContinuationOption(limit)
}

// DefaultSortOption supplies a default list sort.
type DefaultSortOption string

// ApplyToList fills Sort when it is empty.
func (option DefaultSortOption) ApplyToList(options *ListOptions) {
	if options.Sort == "" {
		options.Sort = string(option)
	}
}

// DefaultSort supplies a sort only when the caller omitted it.
func DefaultSort(sort string) DefaultSortOption {
	return DefaultSortOption(sort)
}

// ApplyListOptions expands public list options in declaration order.
func ApplyListOptions(options []ListOption) ListOptions {
	resolved := ListOptions{}
	for _, option := range options {
		option.ApplyToList(&resolved)
	}
	return resolved
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

// ParseSort parses a sort query string. Directions follow the field name,
// such as "name-,time+". The prefix form "-name,+time" is also accepted.
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
		switch s[0] {
		case '-':
			direction = SortDirectionDesc
			s = s[1:]
		case '+':
			direction = SortDirectionAsc
			s = s[1:]
		}
		// Parse the standard suffix form, such as "name-".
		if direction == SortDirectionUnknown && s != "" {
			switch s[len(s)-1] {
			case '-':
				direction = SortDirectionDesc
				s = s[:len(s)-1]
			case '+':
				direction = SortDirectionAsc
				s = s[:len(s)-1]
			}
		}
		if s == "" {
			continue
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

// ObjectMetadata represents the metadata of an object
// it commonly used in API objects
type ObjectMetadata struct {
	// ID is the unique identifier of the object
	// it must not be changed once created
	ID string `json:"id,omitempty"`
	// Name is the name of the object
	// it's used for display only, can be changed to anything
	Name string `json:"name,omitempty"`
	// CreationTimestamp is the creation timestamp of the object
	CreationTimestamp Time `json:"creationTimestamp,omitempty"`
	// ResourceVersion identifies the persisted version used for concurrency.
	ResourceVersion int64 `json:"resourceVersion,omitempty"`
	// DeletionTimestamp is the deletion timestamp of the object
	DeletionTimestamp *Time `json:"deletionTimestamp,omitempty"`
	// Labels is a set of key/value labels for the object
	Labels map[string]string `json:"labels,omitempty"`
	// Annotations is a set of key/value annotations for the object
	Annotations map[string]string `json:"annotations,omitempty"`
	// Description is the description of the object
	Description string `json:"description,omitempty"`
}

// Preconditions restrict a mutation to a caller-observed object identity.
type Preconditions struct {
	// UID prevents a mutation from targeting a replacement object with the same ID.
	UID string `json:"uid,omitempty"`
	// ResourceVersion is optional; zero means no caller-supplied version condition.
	ResourceVersion int64 `json:"resourceVersion,omitempty"`
}
