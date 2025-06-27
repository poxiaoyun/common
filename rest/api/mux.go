package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"xiaoshiai.cn/common/rest/matcher"
)

func MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "405 method not allowed", http.StatusMethodNotAllowed)
}

func UnsupportedMediaType(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "415 unsupported media type", http.StatusUnsupportedMediaType)
}

// The HyperText Transfer Protocol (HTTP) 406 Not Acceptable client error response code indicates
// that the server cannot produce a response matching the list of acceptable values
// defined in the request's proactive content negotiation headers,
// and that the server is unwilling to supply a default representation.
func NotAcceptable(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "406 not acceptable", http.StatusNotAcceptable)
}

func MediaTypeCheckFunc(accepts, produces []string, handler http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if len(accepts) > 0 && !MatchMIME(r.Header.Get("Content-Type"), accepts) {
			UnsupportedMediaType(w, r)
			return
		}
		if len(produces) > 0 && !MatchMIME(r.Header.Get("Accept"), produces) {
			NotAcceptable(w, r)
			return
		}
		handler.ServeHTTP(w, r)
	}
}

func MatchMIME(accept string, supported []string) bool {
	base, _, _ := strings.Cut(accept, ";")
	accept = strings.TrimSpace(strings.ToLower(base))
	if accept == "" || accept == "*/*" || len(supported) == 0 {
		return true
	}
	for _, s := range supported {
		base, _, _ := strings.Cut(s, ";")
		s = strings.TrimSpace(strings.ToLower(base))
		if s == "*/*" || accept == s {
			return true
		}
	}
	return false
}

type MethodsHandler map[string]http.Handler

func (h MethodsHandler) NotAllowed(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Allow", strings.Join(maps.Keys(h), ","))
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
	} else {
		MethodNotAllowed(w, r)
	}
}

func (h MethodsHandler) selectHandler(r *http.Request) http.Handler {
	if len(h) == 0 {
		return nil
	}
	for _, candidate := range []string{r.Method, ""} {
		if handler, ok := h[candidate]; ok {
			return handler
		}
	}
	return nil
}

type Mux struct {
	NotFound         http.Handler
	MethodNotAllowed http.Handler
	HostsTree        map[string]*matcher.Node[MethodsHandler]
	Tree             *matcher.Node[MethodsHandler]
}

func NewMux() *Mux {
	return &Mux{
		HostsTree: make(map[string]*matcher.Node[MethodsHandler]),
		Tree:      &matcher.Node[MethodsHandler]{},
	}
}

func (m *Mux) Handle(method, pattern string, handler http.Handler) error {
	_, node, err := m.Tree.Get(pattern)
	if err != nil {
		return err
	}
	if node.Value == nil {
		node.Value = MethodsHandler{}
	}
	if _, ok := node.Value[method]; ok {
		return fmt.Errorf("already registered: %s %s", method, pattern)
	}
	node.Value[method] = handler
	return nil
}

func (m *Mux) SetNotFound(handler http.Handler) {
	m.NotFound = handler
}

func (m *Mux) SetMethodNotAllowed(handler http.Handler) {
	m.MethodNotAllowed = handler
}

func (m *Mux) Register(route *Route) error {
	method, pattern := route.Method, route.Path
	if len(route.Hosts) > 0 {
		for _, host := range route.Hosts {
			tree, ok := m.HostsTree[host]
			if !ok {
				tree = &matcher.Node[MethodsHandler]{}
				m.HostsTree[host] = tree
			}
			if err := m.register(route, tree); err != nil {
				return fmt.Errorf("register route %s %s for host %s: %w", method, pattern, host, err)
			}
		}
		return nil
	} else {
		if err := m.register(route, m.Tree); err != nil {
			return fmt.Errorf("register route %s %s: %w", method, pattern, err)
		}
		return nil
	}
}

func (m *Mux) register(route *Route, tree *matcher.Node[MethodsHandler]) error {
	method, pattern := route.Method, route.Path
	sections, node, err := tree.Get(pattern)
	if err != nil {
		return err
	}
	if node.Value == nil {
		node.Value = MethodsHandler{}
	}
	if _, ok := node.Value[method]; ok {
		return fmt.Errorf("already registered: %s %s", method, pattern)
	}
	node.Value[method] = route
	// complete pathparam from sections if not exists
	completePathParam(route, sections)
	return nil
}

func completePathParam(route *Route, sections []matcher.Section) {
	vars := []Param{}
	for _, section := range sections {
		for _, elem := range section {
			if elem.VarName != "" {
				// check already exists
				exists := slices.ContainsFunc(route.Params, func(i Param) bool {
					return i.Name == elem.VarName
				})
				if exists {
					continue
				}
				param := Param{
					Name:     elem.VarName,
					Kind:     ParamKindPath,
					DataType: "string",
				}
				if elem.Validate != nil {
					param.PatternExpr = elem.Validate.String()
				}
				vars = append(vars, param)
			}
		}
	}
	route.Params = append(vars, route.Params...)
	route.Path = matcher.NoRegexpString(sections)
}

func DefaultMatchCandidateFunc(val MethodsHandler, vars []matcher.MatchVar) bool {
	for _, v := range vars {
		// if no matched value,skip
		//
		// example: /v1/tenants//organizations matched /v1/tenants/{tenant}/organizations
		// but tenant is empty which is not allowed
		if v.Value == "" {
			return false
		}
	}
	// only match if the node has a handler
	return val != nil
}

func (m *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	matchpath := r.URL.Path
	if r.URL.RawPath != "" {
		matchpath = r.URL.RawPath
	}
	node, vars := m.Tree.Match(matchpath, DefaultMatchCandidateFunc)
	if node == nil || node.Value == nil {
		if m.NotFound == nil {
			http.NotFound(w, r)
		} else {
			m.NotFound.ServeHTTP(w, r)
		}
		return
	}
	reqvars := make([]PathVar, len(vars))
	for i, v := range vars {
		reqvars[i] = PathVar{Key: v.Name, Value: v.Value}
	}
	r = r.WithContext(context.WithValue(r.Context(), httpVarsContextKey{}, reqvars))

	if handler := node.Value.selectHandler(r); handler != nil {
		handler.ServeHTTP(w, r)
		return
	}
	if m.MethodNotAllowed != nil {
		m.MethodNotAllowed.ServeHTTP(w, r)
		return
	}
	node.Value.NotAllowed(w, r)
}

type httpVarsContextKey struct{}

func PathVars(r *http.Request) PathVarList {
	if vars, ok := r.Context().Value(httpVarsContextKey{}).([]PathVar); ok {
		return vars
	}
	return nil
}
