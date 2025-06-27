// Copyright 2023 The Kubegems Authors
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
	"context"
	"net/http"
)

type API struct {
	plugins       []Plugin
	mux           Router
	globalFilters Filters
	registered    []*Route
}

type Router interface {
	Register(route *Route) error
	ServeHTTP(w http.ResponseWriter, r *http.Request)
	SetNotFound(handler http.Handler)
}

func New() *API {
	return &API{mux: NewMux()}
}

func (m *API) Route(route Route) *API {
	if err := m.mux.Register(&route); err != nil {
		panic(err)
	}
	m.registered = append(m.registered, &route)
	for _, plugin := range m.plugins {
		if routePlugin, ok := plugin.(RoutePlugin); ok {
			if err := routePlugin.OnRoute(&route); err != nil {
				panic(err)
			}
		}
	}
	return m
}

func (m *API) NotFound(handler http.Handler) *API {
	m.mux.SetNotFound(handler)
	return m
}

func (m *API) Group(groups ...Group) *API {
	for _, group := range groups {
		for _, route := range group.Build() {
			m.Route(route)
		}
	}
	return m
}

func (m *API) Plugin(plugin ...Plugin) *API {
	for _, p := range plugin {
		if err := p.Install(m); err != nil {
			panic(err)
		}
		m.plugins = append(m.plugins, p)
	}
	return m
}

func (m *API) Filter(filters ...Filter) *API {
	m.globalFilters = append(m.globalFilters, filters...)
	return m
}

func (m *API) Build() http.Handler {
	for _, plugin := range m.plugins {
		if routePlugin, ok := plugin.(RoutePlugin); ok {
			if err := routePlugin.OnBuild(m, m.registered); err != nil {
				panic(err)
			}
		}
	}

	if len(m.globalFilters) > 0 {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			m.globalFilters.Process(w, r, m.mux)
		})
	}
	return m.mux
}

func (m *API) Serve(ctx context.Context, listen string) error {
	return ServeContext(ctx, listen, m.Build())
}

func (m *API) ServeTLS(ctx context.Context, listen, cert, key string) error {
	return ServeContext(ctx, listen, m.Build(), WithDynamicTLSConfig(ctx, cert, key))
}
