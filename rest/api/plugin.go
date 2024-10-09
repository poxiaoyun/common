package api

import (
	"net/http"
)

type Plugin interface {
	Install(m *API) error
}

type RoutePlugin interface {
	Plugin
	OnRoute(route *Route) error
	OnBuild(m *API, routes []*Route) error
}

type VersionPlugin struct {
	Version any
}

func (v VersionPlugin) Install(m *API) error {
	m.Route(GET("/version").Doc("version").To(func(resp http.ResponseWriter, req *http.Request) {
		Raw(resp, http.StatusOK, v.Version, nil)
	}))
	return nil
}

type HealthCheckPlugin struct {
	CheckFun func() error
}

func (h HealthCheckPlugin) Install(m *API) error {
	m.Route(GET("/healthz").Doc("health check").To(func(resp http.ResponseWriter, req *http.Request) {
		if h.CheckFun != nil {
			if err := h.CheckFun(); err != nil {
				ServerError(resp, err)
				return
			}
		}
		Raw(resp, http.StatusOK, "ok", nil)
	}))
	return nil
}
