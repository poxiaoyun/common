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
	"encoding/json"
	"errors"
	"io"
	"net/http"

	liberrors "xiaoshiai.cn/common/errors"
)

// WrapError is a function that wraps the error to be returned in the response.
// It turns the error into a struct that contains the status code, message, and the raw error.
var WrapError = func(data *liberrors.Status) any {
	return data
}

// WrapOK is a function that wraps the data to be returned in the response.
// the common use case is to wrap the data in a struct that contains the data and the status code.
//
//	e.g.:
//	{"foo": "bar"}  =>  {"data": {"foo": "bar"}}
var WrapOK = func(data any) any {
	return data
}

func Success(w http.ResponseWriter, data any) {
	Raw(w, http.StatusOK, WrapOK(data))
}

func NotFound(w http.ResponseWriter, message string) {
	Error(w, liberrors.NewNotFound("", message))
}

func BadRequest(w http.ResponseWriter, message string) {
	Error(w, liberrors.NewBadRequest(message))
}

func Unauthorized(w http.ResponseWriter, message string) {
	Error(w, liberrors.NewUnauthorized(message))
}

func Forbidden(w http.ResponseWriter, message string) {
	Error(w, liberrors.NewForbidden(errors.New(message)))
}

func ServerError(w http.ResponseWriter, err error) {
	Error(w, liberrors.NewInternalError(err))
}

func Error(w http.ResponseWriter, err error) {
	statuse := &liberrors.Status{}
	if !errors.As(err, &statuse) {
		statuse = liberrors.NewBadRequest(err.Error())
	}
	Raw(w, int(statuse.Code), WrapError(statuse))
}

func Raw(w http.ResponseWriter, status int, data any) {
	switch val := data.(type) {
	case io.Reader:
		setContentTypeIfNotSet(w.Header(), "application/octet-stream")
		w.WriteHeader(status)
		_, _ = io.Copy(w, val)
	case string:
		setContentTypeIfNotSet(w.Header(), "text/plain")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(val))
	case []byte:
		setContentTypeIfNotSet(w.Header(), "application/octet-stream")
		w.WriteHeader(status)
		_, _ = w.Write(val)
	case ResponseStatusOnly:
		w.WriteHeader(int(val))
	case nil:
		w.WriteHeader(status)
		// do not write a nil representation
	default:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(data)
	}
}

func setContentTypeIfNotSet(hds http.Header, val string) {
	if hds.Get("Content-Type") == "" {
		hds.Set("Content-Type", val)
	}
}

func Redirect(w http.ResponseWriter, r *http.Request) {
	var queryPart string
	if len(r.URL.RawQuery) > 0 {
		queryPart = "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, r.URL.Path+"/"+queryPart, http.StatusMovedPermanently)
}

func RenderHTML(w http.ResponseWriter, html []byte) {
	w.Header().Set("Content-Type", "text/html")
	w.Write(html)
}
