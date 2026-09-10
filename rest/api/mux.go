package api

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"

	"xiaoshiai.cn/common/rest/matcher"
)

func MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "405 method not allowed", http.StatusMethodNotAllowed)
}

type routeCandidate struct {
	route        *Route
	contentTypes []mediaRange
	accepts      []mediaRange
}

type routeCandidates map[string][]routeCandidate

// Mux selects routes before invoking their filters and handlers. Register routes
// before serving requests; route matching conditions must not change afterward.
type Mux struct {
	NotFound         http.Handler
	MethodNotAllowed http.Handler
	hosts            map[string]*matcher.Node[routeCandidates]
	paths            matcher.Node[routeCandidates]
}

var _ Router = &Mux{}

func NewMux() *Mux {
	return &Mux{}
}

func (m *Mux) Handle(method, pattern string, handler http.Handler) error {
	return m.Register(&Route{Method: method, Path: pattern, Handler: handler})
}

func (m *Mux) SetNotFound(handler http.Handler) {
	m.NotFound = handler
}

func (m *Mux) SetMethodNotAllowed(handler http.Handler) {
	m.MethodNotAllowed = handler
}

func (m *Mux) Register(route *Route) error {
	if route.buildError != nil {
		return fmt.Errorf("register route %s %s: %w", route.Method, route.Path, route.buildError)
	}
	contentTypes, normalizedContentTypes, err := compileMediaRanges(route.ContentTypes)
	if err != nil {
		return fmt.Errorf("register route %s %s Content-Type: %w", route.Method, route.Path, err)
	}
	accepts, normalizedAccepts, err := compileMediaRanges(route.Accepts)
	if err != nil {
		return fmt.Errorf("register route %s %s Accept: %w", route.Method, route.Path, err)
	}
	route.ContentTypes, route.Accepts = normalizedContentTypes, normalizedAccepts
	candidate := routeCandidate{route: route, contentTypes: contentTypes, accepts: accepts}
	pattern := route.Path
	sections, err := matcher.CompilePattern(pattern)
	if err != nil {
		return fmt.Errorf("register route %s %s: %w", route.Method, pattern, err)
	}
	if len(route.Hosts) == 0 {
		if err := m.register(pattern, candidate, &m.paths); err != nil {
			return err
		}
	}
	for _, host := range route.Hosts {
		if m.hosts == nil {
			m.hosts = map[string]*matcher.Node[routeCandidates]{}
		}
		tree := m.hosts[host]
		if tree == nil {
			tree = &matcher.Node[routeCandidates]{}
			m.hosts[host] = tree
		}
		if err := m.register(pattern, candidate, tree); err != nil {
			return fmt.Errorf("host %s: %w", host, err)
		}
	}
	completePathParam(route, sections)
	return nil
}

func (m *Mux) register(pattern string, candidate routeCandidate, tree *matcher.Node[routeCandidates]) error {
	_, node, err := tree.Register(pattern)
	if err != nil {
		return fmt.Errorf("register route %s %s: %w", candidate.route.Method, pattern, err)
	}
	if node.Value == nil {
		node.Value = routeCandidates{}
	}
	method := candidate.route.Method
	for _, existing := range node.Value[method] {
		if sameMediaConditions(existing.route.ContentTypes, candidate.route.ContentTypes) && sameMediaConditions(existing.route.Accepts, candidate.route.Accepts) {
			return fmt.Errorf("already registered: %s %s with Content-Type %v and Accept %v", method, pattern, candidate.route.ContentTypes, candidate.route.Accepts)
		}
	}
	node.Value[method] = append(node.Value[method], candidate)
	return nil
}

func sameMediaConditions(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, value := range a {
		if !slices.Contains(b, value) {
			return false
		}
	}
	return true
}

func completePathParam(route *Route, sections []matcher.Section) {
	vars := []Param{}
	for _, section := range sections {
		for _, elem := range section {
			if elem.VarName != "" {
				// check already exists
				exists := slices.ContainsFunc(route.Params, func(i Param) bool {
					return i.Kind == ParamKindPath && i.Name == elem.VarName
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

func (m *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	matchpath := r.URL.Path
	if r.URL.RawPath != "" {
		matchpath = r.URL.RawPath
	}
	host, _, _ := strings.Cut(r.Host, ":")
	tree := &m.paths
	if hostTree, ok := m.hosts[host]; ok {
		tree = hostTree
	}
	var media requestMedia
	var mediaParsed bool
	allowed := map[string]struct{}{}
	resourceMethods := map[string]struct{}{}
	var selected *Route
	var selectedVars []matcher.MatchVar
	var varyAccept, varyContentType bool
	tree.Match(matchpath, func(candidates routeCandidates, vars []matcher.MatchVar) bool {
		if len(candidates) == 0 {
			return false
		}
		for method, variants := range candidates {
			if method != "" {
				resourceMethods[method] = struct{}{}
			}
			for _, candidate := range variants {
				varyAccept = varyAccept || len(candidate.accepts) > 0
				varyContentType = varyContentType || len(candidate.contentTypes) > 0
				if !mediaParsed && (varyAccept || varyContentType) {
					media = parseRequestMedia(r.Header)
					mediaParsed = true
				}
				if _, ok := media.match(candidate.contentTypes, candidate.accepts); ok && method != "" {
					allowed[method] = struct{}{}
				}
			}
		}
		var pathSelected *Route
		var best mediaMatch
		for _, method := range []string{r.Method, ""} {
			for _, candidate := range candidates[method] {
				match, ok := media.match(candidate.contentTypes, candidate.accepts)
				if !ok {
					continue
				}
				if pathSelected == nil || candidate.route.Priority > pathSelected.Priority ||
					(candidate.route.Priority == pathSelected.Priority && method == pathSelected.Method && match.compare(best) > 0) {
					pathSelected, best = candidate.route, match
				}
			}
		}
		if pathSelected != nil && (selected == nil || pathSelected.Priority > selected.Priority) {
			selected, selectedVars = pathSelected, slices.Clone(vars)
		}
		return false
	})
	header := w.Header()
	if varyAccept {
		addVary(header, "Accept")
	}
	if varyContentType {
		addVary(header, "Content-Type")
	}
	if selected != nil {
		reqvars := make([]PathVar, len(selectedVars))
		for index, variable := range selectedVars {
			reqvars[index] = PathVar{Key: variable.Name, Value: variable.Value}
		}
		r = r.WithContext(context.WithValue(r.Context(), httpVarsContextKey{}, reqvars))
		selected.ServeHTTP(w, r)
		return
	}
	if r.Method == http.MethodOptions && len(resourceMethods) > 0 {
		resourceMethods[http.MethodOptions] = struct{}{}
		header.Set("Allow", strings.Join(slices.Sorted(maps.Keys(resourceMethods)), ", "))
		w.WriteHeader(http.StatusOK)
		return
	}
	if len(allowed) > 0 {
		header.Set("Allow", strings.Join(slices.Sorted(maps.Keys(allowed)), ", "))
		if m.MethodNotAllowed != nil {
			m.MethodNotAllowed.ServeHTTP(w, r)
		} else {
			MethodNotAllowed(w, r)
		}
		return
	}
	if m.NotFound != nil {
		m.NotFound.ServeHTTP(w, r)
	} else {
		http.NotFound(w, r)
	}
}

func addVary(header http.Header, field string) {
	for _, value := range header.Values("Vary") {
		for existing := range strings.SplitSeq(value, ",") {
			existing = strings.TrimSpace(existing)
			if existing == "*" || strings.EqualFold(existing, field) {
				return
			}
		}
	}
	header.Add("Vary", field)
}

type httpVarsContextKey struct{}

func PathVars(r *http.Request) PathVarList {
	ctx := r.Context()
	if vars, ok := ctx.Value(httpVarsContextKey{}).([]PathVar); ok {
		return vars
	}
	return nil
}
