package openapi

import (
	"embed"
	"encoding/json"
	"net/http"
	"net/url"

	"xiaoshiai.cn/common/rest/api"
)

//go:embed ui/*
var UIFS embed.FS

type OpenAPIUI struct {
	OpenAPIHandler http.HandlerFunc
}

func NewOpenAPIUI(docHandler http.HandlerFunc) OpenAPIUI {
	return OpenAPIUI{OpenAPIHandler: docHandler}
}

func (o OpenAPIUI) Redirect(w http.ResponseWriter, r *http.Request) {
	location := url.URL{
		Path:     r.URL.Path + "/",
		RawQuery: r.URL.RawQuery,
	}
	http.Redirect(w, r, location.String(), http.StatusFound)
}

func (o OpenAPIUI) Index(w http.ResponseWriter, r *http.Request) {
	http.ServeFileFS(w, r, UIFS, "ui/index.html")
}

func (o OpenAPIUI) Resources(w http.ResponseWriter, r *http.Request) {
	path := api.Path(r, "path", "")
	http.ServeFileFS(w, r, UIFS, "ui/static/"+path)
}

func (o OpenAPIUI) Group(prefix string) api.Group {
	return api.NewGroup(prefix).
		Route(
			api.GET("/openapi.json").To(o.OpenAPIHandler).NotDocumented(),
			api.GET("/").
				To(o.Index).
				Produce("text/html").
				NotDocumented(),
			api.GET("").To(o.Redirect).NotDocumented(),
			api.GET("/static/{path}*").To(o.Resources).NotDocumented(),
		)
}

func StaticOpenAPIHandler(fn func(r *http.Request) (any, error)) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		openapi, err := fn(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openapi)
	})
}
