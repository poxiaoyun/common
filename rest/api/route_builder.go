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
	"path"
	"strings"
)

type Route struct {
	Summary        string
	OperationName  string
	Path           string
	Method         string
	Deprecated     bool
	Handler        http.Handler
	Filters        Filters
	Tags           []string
	Consumes       []string
	Produces       []string
	Params         []Param
	Responses      []ResponseInfo
	Properties     map[string]any
	RequestSample  any
	ResponseSample any
}

func (route Route) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fn := route.Handler
	if len(route.Produces) != 0 || len(route.Consumes) != 0 {
		fn = MediaTypeCheckFunc(route.Produces, route.Consumes, route.Handler)
	}
	// init filter context
	r = r.WithContext(SetContextValue(r.Context(), "filter-context-init", struct{}{}))
	route.Filters.Process(w, r, fn)
}

type ResponseInfo struct {
	Code        int
	Headers     map[string]string
	Body        any
	Description string
}

func Do(method string, path string) Route {
	return Route{Method: method, Path: path}
}

func Any(path string) Route {
	return Do("", path)
}

func OPTIONS(path string) Route {
	return Do(http.MethodOptions, path)
}

func HEAD(path string) Route {
	return Do(http.MethodHead, path)
}

func GET(path string) Route {
	return Do(http.MethodGet, path)
}

func POST(path string) Route {
	return Do(http.MethodPost, path)
}

func PUT(path string) Route {
	return Do(http.MethodPut, path)
}

func PATCH(path string) Route {
	return Do(http.MethodPatch, path)
}

func DELETE(path string) Route {
	return Do(http.MethodDelete, path)
}

func (n Route) To(fun http.HandlerFunc) Route {
	n.Handler = fun
	return n
}

func (n Route) Tag(tags ...string) Route {
	n.Tags = append(n.Tags, tags...)
	return n
}

func (n Route) Doc(summary string) Route {
	n.Summary = summary
	return n
}

func (n Route) Operation(operation string) Route {
	n.OperationName = operation
	return n
}

func (n Route) Param(params ...Param) Route {
	n.Params = append(n.Params, params...)
	return n
}

// Accept match request Accept header
func (n Route) Accept(mime ...string) Route {
	n.Produces = append(n.Produces, mime...)
	return n
}

// ContentType match request Content-Type header
func (n Route) ContentType(mime ...string) Route {
	n.Consumes = append(n.Consumes, mime...)
	return n
}

func (n Route) Response(body any, desc ...string) Route {
	n.Responses = append(n.Responses, ResponseInfo{Code: http.StatusOK, Body: body, Description: strings.Join(desc, "")})
	return n
}

func (n Route) ResponseStatus(status int, body any, desc ...string) Route {
	n.Responses = append(n.Responses, ResponseInfo{Code: status, Body: body, Description: strings.Join(desc, "")})
	return n
}

func (n Route) Property(k string, v any) Route {
	if n.Properties == nil {
		n.Properties = make(map[string]any)
	}
	n.Properties[k] = v
	return n
}

// Request defines the request body sample
func (n Route) RequestExample(body any) Route {
	n.RequestSample = body
	return n
}

func (n Route) ResponseExample(body any) Route {
	n.ResponseSample = body
	return n
}

type ParamKind string

const (
	ParamKindPath   ParamKind = "path"
	ParamKindQuery  ParamKind = "query"
	ParamKindHeader ParamKind = "header"
	ParamKindForm   ParamKind = "formData"
	ParamKindBody   ParamKind = "body"
)

type Param struct {
	Name          string
	Kind          ParamKind
	Type          string
	Enum          []any
	Default       any
	IsOptional    bool
	Description   string
	Example       any
	PatternExpr   string
	AllowMultiple bool
}

func BodyParam(name string, value any) Param {
	return Param{Kind: ParamKindBody, Name: name, Example: value}
}

func FormParam(name string, description string) Param {
	return Param{Kind: ParamKindForm, Name: name, Description: description}
}

func PathParam(name string, description string) Param {
	return Param{Kind: ParamKindPath, Type: "string", Name: name, Description: description}
}

func QueryParam(name string, description string) Param {
	return Param{Kind: ParamKindQuery, Type: "string", Name: name, Description: description}
}

func (p Param) Optional() Param {
	p.IsOptional = true
	return p
}

func (p Param) Desc(desc string) Param {
	p.Description = desc
	return p
}

func (p Param) DataType(t string) Param {
	p.Type = t
	return p
}

func (p Param) In(t ...any) Param {
	p.Enum = append(p.Enum, t...)
	return p
}

func (p Param) Def(def string) Param {
	p.Default = def
	return p
}

func (p Param) Pattern(pattern string) Param {
	p.PatternExpr = pattern
	return p
}

func (p Param) Multiple() Param {
	p.AllowMultiple = true
	return p
}

type Group struct {
	Path      string
	Filters   Filters
	Tags      []string
	Params    []Param // common params apply to all routes in the group
	Routes    []Route
	SubGroups []Group // sub groups
	Consumes  []string
	Produces  []string
}

func NewGroup(path string) Group {
	return Group{Path: path}
}

func (g Group) Tag(name string) Group {
	g.Tags = append(g.Tags, name)
	return g
}

// ContentType match request Content-Type header
func (g Group) ContentType(mime ...string) Group {
	g.Consumes = append(g.Consumes, mime...)
	return g
}

// Accept match request Accept header
func (g Group) Accept(mime ...string) Group {
	g.Produces = append(g.Produces, mime...)
	return g
}

func (g Group) Route(rs ...Route) Group {
	g.Routes = append(g.Routes, rs...)
	return g
}

func (g Group) SubGroup(groups ...Group) Group {
	g.SubGroups = append(g.SubGroups, groups...)
	return g
}

func (g Group) Param(params ...Param) Group {
	g.Params = append(g.Params, params...)
	return g
}

func (g Group) Filter(filters ...Filter) Group {
	g.Filters = append(g.Filters, filters...)
	return g
}

func (t Group) Build() map[string]map[string]Route {
	// path -> method -> route
	items := map[string]map[string]Route{}
	buildRoutes(items, Group{}, t)
	return items
}

func buildRoutes(items map[string]map[string]Route, merged Group, group Group) {
	merged.Path = path.Join(merged.Path, group.Path)
	merged.Params = append(merged.Params, group.Params...)
	merged.Tags = append(merged.Tags, group.Tags...)
	merged.Consumes = append(merged.Consumes, group.Consumes...)
	merged.Produces = append(merged.Produces, group.Produces...)
	merged.Filters = append(merged.Filters, group.Filters...)

	for _, route := range group.Routes {
		route.Tags = append(merged.Tags, route.Tags...)
		route.Params = append(merged.Params, route.Params...)
		route.Path = merged.Path + route.Path
		route.Consumes = append(group.Consumes, route.Consumes...)
		route.Produces = append(group.Produces, route.Produces...)
		route.Filters = append(merged.Filters, route.Filters...)
		pathmethods, ok := items[route.Path]
		if !ok {
			pathmethods = map[string]Route{}
			items[route.Path] = pathmethods
		}
		pathmethods[route.Method] = route
	}
	for _, group := range group.SubGroups {
		buildRoutes(items, merged, group)
	}
}
