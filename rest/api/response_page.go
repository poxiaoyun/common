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
)

const DefaultPageSize = 10

type Page[T any] struct {
	Total int64 `json:"total"`
	Items []T   `json:"items"`
	Page  int64 `json:"page"`
	Size  int64 `json:"size"`
}

func PageObjectFromRequest[T any](req *http.Request, list []T) Page[T] {
	return PageObjectFromListOptions(list, GetListOptions(req))
}

// PageObjectFromListOptions used for client.Object pagination T in list
// use any of T to suit for both eg. Pod(not implement metav1.Object) and *Pod(metav1.Object)
func PageObjectFromListOptions[T any](list []T, opts ListOptions) Page[T] {
	getname := func(t T) string {
		if item, ok := any(t).(interface{ GetName() string }); ok {
			return item.GetName()
		}
		if item, ok := any(&t).(interface{ GetName() string }); ok {
			return item.GetName()
		}
		return ""
	}
	gettime := func(t T) time.Time {
		if item, ok := any(t).(interface{ GetCreationTimestamp() metav1.Time }); ok {
			return item.GetCreationTimestamp().Time
		}
		if item, ok := any(&t).(interface{ GetCreationTimestamp() metav1.Time }); ok {
			return item.GetCreationTimestamp().Time
		}
		return time.Time{}
	}
	return PageFromListOptions(list, opts, getname, gettime)
}

// PageFromRequest auto pagination from user request on item name or time in list
func PageFromRequest[T any](req *http.Request, list []T, namefunc func(item T) string, timefunc func(item T) time.Time) Page[T] {
	return PageFromListOptions(list, GetListOptions(req), namefunc, timefunc)
}

func PageFromListOptions[T any](list []T, opts ListOptions, namefunc func(item T) string, timefunc func(item T) time.Time) Page[T] {
	return PageFrom(list, opts.Page, opts.Size, SearchNameFunc(opts.Search, namefunc), SortByFunc(opts.Sort, namefunc, timefunc))
}

// PageFrom auto pagination from list with page, size, pickfun and sortfun
// if size is 0, return all items(no pagination)
func PageFrom[T any](list []T, page, size int, pickfun func(item T) bool, sortfun func(a, b T) int) Page[T] {
	// filter
	if pickfun != nil {
		datas := []T{}
		for _, item := range list {
			if pickfun(item) {
				datas = append(datas, item)
			}
		}
		list = datas
	}
	// sort
	if sortfun != nil {
		slices.SortFunc(list, sortfun)
	}
	// page
	if size == 0 {
		return Page[T]{Total: int64(len(list)), Items: list}
	}
	if page < 1 {
		page = 1
	}
	total := len(list)
	startIdx := (page - 1) * size
	endIdx := startIdx + size
	if startIdx > total {
		startIdx = 0
		endIdx = 0
	}
	if endIdx > total {
		endIdx = total
	}
	list = list[startIdx:endIdx]
	return Page[T]{
		Total: int64(total),
		Items: list,
		Page:  int64(page),
		Size:  int64(size),
	}
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
