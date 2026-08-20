// Copyright 2022 The kubegems.io Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package api

import (
	"net/http"
	"strings"
	"time"

	"golang.org/x/exp/slices"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/meta"
)

type pageObjectStringUID interface {
	// GetUID returns the stable unique identity used as a continuation cursor.
	GetUID() string
}

type pageObjectKubernetesUID interface {
	// GetUID returns the stable Kubernetes identity used as a continuation cursor.
	GetUID() types.UID
}

type pageObjectName interface {
	// GetName returns the object name used by list search and ordering.
	GetName() string
}

type pageObjectCreationTimestamp interface {
	// GetCreationTimestamp returns the creation time used by list ordering.
	GetCreationTimestamp() metav1.Time
}

// Page is the shared flat list response contract.
type Page[T any] = meta.Page[T]

// PageObjectFromRequest parses a page request, filters and sorts objects by
// their metadata, and returns the selected page. Continuation uses object UIDs;
// an unavailable cursor returns ResourceExpired. Filtering and sorting reuse
// the input slice's backing array.
func PageObjectFromRequest[T any](req *http.Request, list []T) (Page[T], error) {
	options, err := GetListOptions(req)
	if err != nil {
		return Page[T]{}, err
	}
	return PageObjectFromListOptions(list, options)
}

// PageObjectFromListOptions filters and sorts objects by their metadata and
// applies trusted list options. T may expose metadata methods as either T or
// *T. Limit takes precedence over a positive Size; fields outside the selected
// pagination behavior are ignored. Continuation requires stable,
// unique, non-empty object UIDs and returns ResourceExpired when its cursor is
// absent from the current list. Filtering and sorting reuse the input slice's
// backing array.
func PageObjectFromListOptions[T any](list []T, opts ListOptions) (Page[T], error) {
	getID := func(t T) string {
		if item, ok := valueOrPointerAs[pageObjectStringUID](t); ok {
			return item.GetUID()
		}
		if item, ok := valueOrPointerAs[pageObjectKubernetesUID](t); ok {
			return string(item.GetUID())
		}
		return ""
	}
	getname := func(t T) string {
		if item, ok := valueOrPointerAs[pageObjectName](t); ok {
			return item.GetName()
		}
		return ""
	}
	gettime := func(t T) time.Time {
		if item, ok := valueOrPointerAs[pageObjectCreationTimestamp](t); ok {
			return item.GetCreationTimestamp().Time
		}
		return time.Time{}
	}
	return PageFromListOptions(list, opts, getID, getname, gettime)
}

func valueOrPointerAs[I any, T any](value T) (I, bool) {
	if result, ok := any(value).(I); ok {
		return result, true
	}
	result, ok := any(&value).(I)
	return result, ok
}

// PageFromRequest parses a page request, applies the supplied search and sort
// projections, and returns the selected page. Continuation uses the stable,
// unique, non-empty value returned by getID; an unavailable cursor returns
// ResourceExpired. Filtering and sorting reuse the input slice's backing
// array.
func PageFromRequest[T any](
	req *http.Request,
	list []T,
	getID func(item T) string,
	getName func(item T) string,
	getTime func(item T) time.Time,
) (Page[T], error) {
	options, err := GetListOptions(req)
	if err != nil {
		return Page[T]{}, err
	}
	return PageFromListOptions(list, options, getID, getName, getTime)
}

// PageFromListOptions applies search, sort, and trusted pagination options.
// Limit takes precedence over a positive Size; fields outside the selected
// pagination behavior are ignored. Continuation requires getID
// to return stable, unique, non-empty values and returns ResourceExpired when
// its cursor is absent from the current list. Filtering and sorting reuse the
// input slice's backing array.
func PageFromListOptions[T any](
	list []T,
	opts ListOptions,
	getID func(item T) string,
	getName func(item T) string,
	getTime func(item T) time.Time,
) (Page[T], error) {
	list = filteredAndSortedList(list, SearchNameFunc(opts.Search, getName), SortByFunc(opts.Sort, getName, getTime))
	if opts.Limit > 0 {
		return continuationPageFromPreparedList(list, opts.Continue, opts.Limit, getID)
	}
	if opts.Size > 0 {
		return PageFromPreparedList(list, opts.Page, opts.Size), nil
	}
	total := len(list)
	return Page[T]{Total: &total, Items: list}, nil
}

// PageFrom filters, sorts, and applies trusted page/size pagination. Page
// values below one are treated as one. Size values below zero are treated as
// zero, and a zero size returns all items without pagination. Filtering and
// sorting reuse the input slice's backing array.
func PageFrom[T any](list []T, page, size int, pickfun func(item T) bool, sortfun func(a, b T) int) Page[T] {
	list = filteredAndSortedList(list, pickfun, sortfun)
	return PageFromPreparedList(list, page, size)
}

func filteredAndSortedList[T any](list []T, pickfun func(item T) bool, sortfun func(a, b T) int) []T {
	if pickfun != nil {
		// Compact matching items in place without clearing the unused tail, so
		// callers retaining the original slice length do not observe zeroed items.
		filtered := list[:0]
		for _, item := range list {
			if pickfun(item) {
				filtered = append(filtered, item)
			}
		}
		list = filtered
	}
	// sort
	if sortfun != nil {
		slices.SortFunc(list, sortfun)
	}
	return list
}

// PageFromPreparedList applies trusted page/size pagination to an already
// filtered and sorted list. Page values below one are treated as one. Size
// values below zero are treated as zero, and zero returns all items.
func PageFromPreparedList[T any](list []T, page, size int) Page[T] {
	total := len(list)
	page = max(page, 1)
	size = max(size, 0)
	if size == 0 {
		return Page[T]{Total: &total, Items: list}
	}
	pageIndex := page - 1
	startIdx := total
	if pageIndex <= total/size {
		startIdx = pageIndex * size
	}
	endIdx := startIdx + min(size, total-startIdx)
	list = list[startIdx:endIdx]
	return Page[T]{
		Total: &total,
		Items: list,
		Page:  page,
		Size:  size,
	}
}

func continuationPageFromPreparedList[T any](list []T, continueToken string, limit int, getID func(item T) string) (Page[T], error) {
	if getID == nil {
		return Page[T]{}, errors.NewUnsupported("continuation pagination is not supported for this resource")
	}
	start := 0
	if continueToken != "" {
		start = -1
		for index, item := range list {
			if getID(item) == continueToken {
				start = index + 1
				break
			}
		}
		if start < 0 {
			return Page[T]{}, errors.NewResourceExpired("", "continue token is no longer present")
		}
	}
	end := start + min(limit, len(list)-start)
	items := list[start:end]
	nextToken := ""
	if end < len(list) {
		nextToken = getID(items[len(items)-1])
		if nextToken == "" {
			return Page[T]{}, errors.NewUnsupported("continuation pagination is not supported for this resource")
		}
	}
	return Page[T]{Items: items, Continue: nextToken, Limit: limit}, nil
}

// SearchNameFunc returns a case-sensitive name predicate for search.
func SearchNameFunc[T any](search string, getname func(T) string) func(T) bool {
	if getname == nil || search == "" {
		return nil
	}
	return func(item T) bool {
		return strings.Contains(getname(item), search)
	}
}

// SortByFunc returns a comparator for the supported name and creation-time
// sort expressions.
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
	case "createTimeDesc", "creationTimestamp-", "time-", "": // default sort by time desc
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
