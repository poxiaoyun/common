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
	"fmt"
	"net/http"
	"strings"
)

type Route struct {
	SummaryText    string
	Description    string
	Path           string
	Method         string
	Priority       int      // higher values precede other matching routes; zero preserves path specificity
	Hosts          []string // request must match this host
	IsDeprecated   bool
	Handler        http.Handler
	Filters        Filters
	Tags           []string
	ContentTypes   []string // request Content-Type alternatives; empty means unrestricted
	Accepts        []string // response media types matched against Accept; empty means unrestricted
	Params         []Param
	Responses      []ResponseInfo
	Properties     map[string]any
	RequestSample  any
	ResponseSample any
	NotDoc         bool // if true, this route will not be documented in OpenAPI
	buildError     error
}

func (route Route) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// init filter context
	r = r.WithContext(SetContextValue(r.Context(), "filter-context-init", struct{}{}))
	route.Filters.Process(w, r, route.Handler)
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

// Desc sets the human-readable description of the route.
func (n Route) Desc(desc string) Route {
	n.Description = desc
	return n
}

// Summary sets the human-readable summary of the route.
func (n Route) Summary(summary string) Route {
	n.SummaryText = summary
	return n
}

// Doc is kept as a compatibility alias for Summary.
// Deprecated: use Summary.
func (n Route) Doc(summary string) Route {
	return n.Summary(summary)
}

// Operation is kept as a compatibility alias for Summary.
// Deprecated: use Summary.
func (n Route) Operation(summary string) Route {
	return n.Summary(summary)
}

func (n Route) Deprecated() Route {
	n.IsDeprecated = true
	return n
}

func (n Route) Param(params ...Param) Route {
	n.Params = append(n.Params, params...)
	return n
}

// Accept selects this route when Accept permits one of the declared response media types.
func (n Route) Accept(mediaTypes ...string) Route {
	n.Accepts = append(n.Accepts, mediaTypes...)
	return n
}

// ContentType selects this route when Content-Type matches one of the declared media ranges.
func (n Route) ContentType(mediaTypes ...string) Route {
	n.ContentTypes = append(n.ContentTypes, mediaTypes...)
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

func (n Route) Host(host ...string) Route {
	n.Hosts = append(n.Hosts, host...)
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

func (n Route) NotDocumented() Route {
	n.NotDoc = true
	return n
}

type ParamKind string

const (
	ParamKindPath   ParamKind = "path"
	ParamKindQuery  ParamKind = "query"
	ParamKindHeader ParamKind = "header"
	ParamKindForm   ParamKind = "form"
	ParamKindBody   ParamKind = "body"
)

type Param struct {
	Name          string
	Kind          ParamKind
	DataType      string
	DataFormat    string
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
	return Param{Kind: ParamKindPath, DataType: "string", Name: name, Description: description}
}

func QueryParam(name string, description string) Param {
	return Param{Kind: ParamKindQuery, DataType: "string", Name: name, Description: description}
}

func HeaderParam(name string, description string) Param {
	return Param{Kind: ParamKindHeader, Name: name, Description: description}
}

func (p Param) Optional() Param {
	p.IsOptional = true
	return p
}

func (p Param) Desc(desc string) Param {
	p.Description = desc
	return p
}

func (p Param) Type(t string) Param {
	p.DataType = t
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

func (p Param) Format(format string) Param {
	p.DataFormat = format
	return p
}

func (p Param) Multiple() Param {
	p.AllowMultiple = true
	return p
}

type Group struct {
	Path         string
	IsDeprcated  bool
	Filters      Filters
	Hosts        []string // request must match this host
	Tags         []string
	Params       []Param // common params apply to all routes in the group
	Routes       []Route
	SubGroups    []Group // sub groups
	ContentTypes []string
	Accepts      []string
}

func NewGroup(path string) Group {
	return Group{Path: path}
}

func (g Group) Tag(name string) Group {
	g.Tags = append(g.Tags, name)
	return g
}

func (g Group) Host(host ...string) Group {
	g.Hosts = append(g.Hosts, host...)
	return g
}

// ContentType constrains child routes to these request media ranges.
// A child's media conditions intersect its ancestors' conditions.
func (g Group) ContentType(mediaTypes ...string) Group {
	g.ContentTypes = append(g.ContentTypes, mediaTypes...)
	return g
}

// Accept constrains child routes to these acceptable response media types.
// A child's media conditions intersect its ancestors' conditions.
func (g Group) Accept(mediaTypes ...string) Group {
	g.Accepts = append(g.Accepts, mediaTypes...)
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

func (g Group) Deprecated() Group {
	g.IsDeprcated = true
	return g
}

func (g Group) Filter(filters ...Filter) Group {
	g.Filters = append(g.Filters, filters...)
	return g
}

func (t Group) Build() []Route {
	return buildRoutes(Group{}, t, nil)
}

func buildRoutes(merged Group, group Group, buildError error) []Route {
	if group.Path != "" {
		if merged.Path == "" {
			merged.Path = group.Path
		} else {
			merged.Path = strings.TrimSuffix(merged.Path, "/") + "/" + strings.TrimPrefix(group.Path, "/")
		}
	}
	merged.Params = append(merged.Params, group.Params...)
	merged.Tags = append(merged.Tags, group.Tags...)
	var err error
	merged.ContentTypes, err = intersectMediaConditions(merged.ContentTypes, group.ContentTypes)
	if err != nil && buildError == nil {
		buildError = fmt.Errorf("group %s Content-Type: %w", merged.Path, err)
	}
	merged.Accepts, err = intersectMediaConditions(merged.Accepts, group.Accepts)
	if err != nil && buildError == nil {
		buildError = fmt.Errorf("group %s Accept: %w", merged.Path, err)
	}
	merged.Filters = append(merged.Filters, group.Filters...)
	merged.IsDeprcated = merged.IsDeprcated || group.IsDeprcated
	merged.Hosts = append(merged.Hosts, group.Hosts...)

	var ret []Route
	for _, route := range group.Routes {
		route.Tags = append(merged.Tags, route.Tags...)
		route.Params = append(merged.Params, route.Params...)
		prefix := merged.Path
		if strings.HasPrefix(route.Path, "/") {
			prefix = strings.TrimSuffix(prefix, "/")
		}
		route.Path = prefix + route.Path
		if !strings.HasPrefix(route.Path, "/") {
			route.Path = "/" + route.Path
		}
		if buildError != nil {
			route.buildError = buildError
		}
		route.ContentTypes, err = intersectMediaConditions(merged.ContentTypes, route.ContentTypes)
		if err != nil && route.buildError == nil {
			route.buildError = fmt.Errorf("Content-Type: %w", err)
		}
		route.Accepts, err = intersectMediaConditions(merged.Accepts, route.Accepts)
		if err != nil && route.buildError == nil {
			route.buildError = fmt.Errorf("Accept: %w", err)
		}
		route.Filters = append(merged.Filters, route.Filters...)
		route.Hosts = append(merged.Hosts, route.Hosts...)
		route.IsDeprecated = route.IsDeprecated || group.IsDeprcated
		ret = append(ret, route)
	}
	for _, group := range group.SubGroups {
		ret = append(ret, buildRoutes(merged, group, buildError)...)
	}
	return ret
}
