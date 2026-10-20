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
	stderrors "errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"reflect"
	"strings"

	yaml "sigs.k8s.io/yaml/goyaml.v2"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/meta"
	libreflect "xiaoshiai.cn/common/reflect"
)

// PageParams documents the flat page/size and continue/limit query contract.
// Empty values are treated as omitted.
var PageParams = []Param{
	QueryParam("page", "one-based page number; values below one select the first page").Optional(),
	QueryParam("size", "positive number of items in a page").Optional(),
	QueryParam("search", "Search string for searching").Optional(),
	QueryParam("sort", "Sort string for sorting").In("name", "name-", "time", "time-").Optional(),
	QueryParam("labelSelector", "Selector string for filtering").Optional(),
	QueryParam("fieldSelector", "Selector string for filtering").Optional(),
	QueryParam("continue", "opaque token for a later continuation batch").Optional(),
	QueryParam("limit", "positive maximum number of items in a continuation batch").Optional(),
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

// ListOptions is the shared flat list request contract.
type ListOptions = meta.ListOptions

// GetListOptions parses the flat list query contract, then applies caller-owned
// options in declaration order. Empty query values parse to their zero values,
// so Default options can fill them. Pagination behavior is selected by the
// service that executes the list operation. The error result is reserved for
// request parsing rules; pagination selection itself does not return an error.
func GetListOptions(r *http.Request, modifiers ...meta.ListOption) (ListOptions, error) {
	queries := r.URL.Query()
	options := ListOptions{}
	if value := queries.Get("page"); value != "" {
		options.Page = ValueOrDefault(value, 0)
	}
	if value := queries.Get("size"); value != "" {
		options.Size = ValueOrDefault(value, 0)
	}
	options.Search = queries.Get("search")
	options.Sort = queries.Get("sort")
	options.Continue = queries.Get("continue")
	if value := queries.Get("limit"); value != "" {
		options.Limit = ValueOrDefault(value, 0)
	}
	fieldSelector := queries.Get("fieldSelector")
	if fieldSelector == "" {
		fieldSelector = queries.Get("field-selector")
	}
	labelSelector := queries.Get("labelSelector")
	if labelSelector == "" {
		labelSelector = queries.Get("label-selector")
	}
	options.FieldSelector = fieldSelector
	options.LabelSelector = labelSelector
	for _, modifier := range modifiers {
		modifier.ApplyToList(&options)
	}
	return options, nil
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
	cached := GetContextValue[*url.Values](r.Context(), "queries")
	if cached == nil {
		// cache queries
		queries := r.URL.Query()
		cached = &queries
		ctx := SetContextValue(r.Context(), "queries", cached)
		*r = *r.WithContext(ctx)
	}
	val := cached.Get(key)
	return ValueOrDefault(val, defaultValue)
}

// QueryObjectOptions controls validation of the decoded query shape.
type QueryObjectOptions struct {
	Strict bool
}

// QueryObjectOption configures one QueryObject call.
type QueryObjectOption interface {
	// ApplyToQueryObject configures this call's query validation.
	ApplyToQueryObject(options *QueryObjectOptions)
}

// StrictQueryOption rejects unknown keys and malformed URL query encoding.
type StrictQueryOption struct{}

// WithStrictQuery makes the decoded structure own the complete query shape.
func WithStrictQuery() StrictQueryOption { return StrictQueryOption{} }

// ApplyToQueryObject enables strict query-shape validation.
func (StrictQueryOption) ApplyToQueryObject(options *QueryObjectOptions) { options.Strict = true }

// ApplyQueryObjectOptions applies query options in declaration order.
func ApplyQueryObjectOptions(options ...QueryObjectOption) QueryObjectOptions {
	var result QueryObjectOptions
	for _, option := range options {
		option.ApplyToQueryObject(&result)
	}
	return result
}

// QueryObject decodes URL query parameters into a struct.
//
// Fields use the query tag, falling back to the json tag. Anonymous structs
// and fields tagged with ",inline" are flattened; named structs use
// dot-separated paths. Unknown keys are ignored unless WithStrictQuery is used.
func QueryObject[T any](r *http.Request, options ...QueryObjectOption) (T, error) {
	var result T
	value := reflect.ValueOf(&result).Elem()
	for value.Kind() == reflect.Pointer {
		value.Set(reflect.New(value.Type().Elem()))
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return result, fmt.Errorf("query object must be a struct, got %s", value.Kind())
	}

	settings := ApplyQueryObjectOptions(options...)
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		if settings.Strict {
			return result, errors.NewBadRequest(fmt.Sprintf("invalid query encoding: %v", err))
		}
	}
	err = libreflect.WalkStructFields(value.Type(), func(name string, index []int, _ bool) error {
		values, ok := query[name]
		if settings.Strict {
			delete(query, name)
		}
		if !ok {
			return nil
		}
		if err := libreflect.SetValue(
			libreflect.FieldByIndexAlloc(value, index),
			values,
		); err != nil {
			return errors.NewBadRequest(fmt.Sprintf("invalid query parameter %q: %v", name, err))
		}
		return nil
	}, "query", "json")
	if err != nil {
		return result, err
	}
	if settings.Strict {
		for name := range query {
			return result, errors.NewBadRequest(fmt.Sprintf("unknown query parameter %q", name))
		}
	}
	return result, nil
}

// nolint: forcetypeassert,gomnd,ifshort
// ValueOrDefault return default value if empty string
func ValueOrDefault[T any](val string, defaultValue T) T {
	return meta.ParseString(val, defaultValue)
}

// BodyOptions controls request body decoding.
type BodyOptions struct {
	StrictJSON bool
	// JSONMediaType selects the strict decoder media type; empty means application/json.
	JSONMediaType string
}

// BodyOption configures request body decoding.
type BodyOption interface {
	// ApplyToBody applies this option to the request's decoding settings.
	ApplyToBody(options *BodyOptions)
}

// StrictBodyOption requires one JSON value with no unknown struct fields.
type StrictBodyOption struct{}

// ApplyToBody enables strict JSON decoding.
func (StrictBodyOption) ApplyToBody(options *BodyOptions) { options.StrictJSON = true }

// WithStrictBody rejects unknown or mistyped JSON fields with 422 and malformed
// or trailing input with 400. Media defaults to application/json and can be selected
// with WithJSONMediaType; mismatches return 415.
func WithStrictBody() StrictBodyOption { return StrictBodyOption{} }

// JSONMediaTypeOption selects the media type accepted by strict JSON decoding.
type JSONMediaTypeOption string

// ApplyToBody configures the media type; strict decoding remains an explicit option.
func (option JSONMediaTypeOption) ApplyToBody(options *BodyOptions) {
	options.JSONMediaType = string(option)
}

// WithJSONMediaType selects a JSON protocol media type, such as
// application/cloudevents+json. Strict decoding defaults to application/json.
func WithJSONMediaType(mediaType string) JSONMediaTypeOption {
	return JSONMediaTypeOption(mediaType)
}

// ApplyBodyOptions expands body options in declaration order.
func ApplyBodyOptions(options []BodyOption) BodyOptions {
	var result BodyOptions
	for _, option := range options {
		option.ApplyToBody(&result)
	}
	return result
}

func Body(r *http.Request, into any, options ...BodyOption) error {
	contentEncoding := r.Header.Get("Content-Encoding")
	contentType := r.Header.Get("Content-Type")
	defer r.Body.Close()
	if r.ContentLength == 0 {
		return errors.NewBadRequest("request body is empty")
	}
	if err := readContent(contentType, contentEncoding, r.Body, into, ApplyBodyOptions(options)); err != nil {
		return err
	}
	return ValidateBody(r, into)
}

// ReadContent decodes a content stream using the same options as Body. Validation
// hooks and request-body closure remain with Body.
func ReadContent(contentType, contentEncoding string, body io.Reader, into any, options ...BodyOption) error {
	return readContent(contentType, contentEncoding, body, into, ApplyBodyOptions(options))
}

func readContent(contentType, contentEncoding string, body io.Reader, into any, options BodyOptions) error {
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
		if options.StrictJSON {
			return errors.NewBadRequest(err.Error())
		}
		return err
	}
	if options.StrictJSON {
		expected := options.JSONMediaType
		if expected == "" {
			expected = "application/json"
		}
		if mediatype != expected {
			return errors.NewCustomError(http.StatusUnsupportedMediaType, errors.StatusReasonUnsupported, expected+" is required")
		}
		decoder := json.NewDecoder(body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(into); err != nil {
			var typeError *json.UnmarshalTypeError
			if stderrors.As(err, &typeError) || strings.HasPrefix(err.Error(), "json: unknown field ") {
				return errors.NewCustomError(http.StatusUnprocessableEntity, errors.StatusReasonInvalid, err.Error())
			}
			return errors.NewBadRequest(err.Error())
		}
		var trailing json.RawMessage
		if err := decoder.Decode(&trailing); err != io.EOF {
			return errors.NewBadRequest("request body must contain exactly one JSON value")
		}
		return nil
	}
	switch mediatype {
	case "application/json":
		return json.NewDecoder(body).
			Decode(into)
	case "application/xml":
		return xml.NewDecoder(body).
			Decode(into)
	case "application/yaml":
		return yaml.NewDecoder(body).
			Decode(into)
	default:
		return json.NewDecoder(body).
			Decode(into)
	}
}
