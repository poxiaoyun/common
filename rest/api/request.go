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
	"compress/gzip"
	"compress/zlib"
	"encoding/json"
	"encoding/xml"
	"io"
	"mime"
	"net/http"

	yaml "sigs.k8s.io/yaml/goyaml.v2"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/meta"
)

var PageParams = []Param{
	QueryParam("limit", "size limit").Optional(),
	QueryParam("size", "size limit").Optional(),
	QueryParam("page", "page number").Optional(),
	QueryParam("search", "Search string for searching").Optional(),
	QueryParam("sort", "Sort string for sorting").In("name", "name-", "time", "time-").Optional(),
	QueryParam("label-selector", "Selector string for filtering").Optional(),
	QueryParam("continue", "Continue token for pagination").Optional(),
}

type PathVar struct {
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
}

type PathVarList []PathVar

func (p PathVarList) Get(key string) string {
	for _, v := range p {
		if v.Key == key {
			return v.Value
		}
	}
	return ""
}

func (p PathVarList) Map() map[string]string {
	m := make(map[string]string, len(p))
	for _, v := range p {
		m[v.Key] = v.Value
	}
	return m
}

type ListOptions = meta.ListOptions

func GetListOptions(r *http.Request) ListOptions {
	return ListOptions{
		Page:     Query(r, "page", 1),
		Size:     Query(r, "size", 10),
		Search:   Query(r, "search", ""),
		Sort:     Query(r, "sort", ""),
		Continue: Query(r, "continue", ""),
	}
}

func HeaderOrQuery[T any](r *http.Request, key string, defaultValue T) T {
	if val := r.Header.Get(key); val == "" {
		return ValueOrDefault(r.URL.Query().Get(key), defaultValue)
	} else {
		return ValueOrDefault(val, defaultValue)
	}
}

func Path[T any](r *http.Request, key string, defaultValue T) T {
	return ValueOrDefault(PathVars(r).Get(key), defaultValue)
}

func Header[T any](r *http.Request, key string, defaultValue T) T {
	val := r.Header.Get(key)
	return ValueOrDefault(val, defaultValue)
}

func Query[T any](r *http.Request, key string, defaultValue T) T {
	val := r.URL.Query().Get(key)
	return ValueOrDefault(val, defaultValue)
}

// nolint: forcetypeassert,gomnd,ifshort
// ValueOrDefault return default value if empty string
func ValueOrDefault[T any](val string, defaultValue T) T {
	return meta.ParseString(val, defaultValue)
}

func Body(r *http.Request, into any) error {
	contentEncoding := r.Header.Get("Content-Encoding")
	contentType := r.Header.Get("Content-Type")
	defer r.Body.Close()
	if r.ContentLength == 0 {
		return errors.NewBadRequest("request body is empty")
	}
	if err := ReadContent(contentType, contentEncoding, r.Body, into); err != nil {
		return err
	}
	return ValidateBody(r, into)
}

func ReadContent(contentType, contentEncoding string, body io.Reader, into any) error {
	// check if the request body needs decompression
	switch contentEncoding {
	case "gzip":
		gzr, err := gzip.NewReader(body)
		if err != nil {
			return err
		}
		defer gzr.Close()
		body = gzr
	case "deflate":
		zlibr, err := zlib.NewReader(body)
		if err != nil {
			return err
		}
		defer zlibr.Close()
		body = zlibr
	}
	mediatype, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return err
	}
	switch mediatype {
	case "application/json":
		return json.NewDecoder(body).Decode(into)
	case "application/xml":
		return xml.NewDecoder(body).Decode(into)
	case "application/yaml":
		return yaml.NewDecoder(body).Decode(into)
	default:
		return json.NewDecoder(body).Decode(into)
	}
}
