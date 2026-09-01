package api

import (
	"context"
	"net/http"
	"strings"

	"xiaoshiai.cn/common/authz"
)

// AttributeResource identifies one resource in an authorization target path.
type AttributeResource struct {
	Resource string `json:"resource,omitempty"`
	Name     string `json:"name,omitempty"`
}

// Attributes describes the operation and resources being authorized.
type Attributes struct {
	// Service is the name of the service that the request is targeting.
	Service   string              `json:"service,omitempty"`
	Method    string              `json:"method,omitempty"`
	Action    string              `json:"action,omitempty"`
	Resources []AttributeResource `json:"resources,omitempty"`
	Path      string              `json:"path,omitempty"`
}

const (
	authorizationContextHTTPMethod = "http.method"
	authorizationContextHTTPPath   = "http.path"
)

func authorizationOperation(attributes Attributes) authz.Operation {
	operation := authz.Operation{
		Service: attributes.Service,
		Action:  attributes.Action,
		Context: authz.Context{
			authorizationContextHTTPMethod: attributes.Method,
			authorizationContextHTTPPath:   attributes.Path,
		},
	}
	if len(attributes.Resources) == 0 {
		operation.Path = attributes.Path
		return operation
	}
	operation.Resource.Scope = make(authz.Scope, len(attributes.Resources)-1)
	for index, resource := range attributes.Resources[:len(attributes.Resources)-1] {
		operation.Resource.Scope[index] = authz.ResourceReference{Type: resource.Resource, ID: resource.Name}
	}
	target := attributes.Resources[len(attributes.Resources)-1]
	operation.Resource.Type = target.Resource
	operation.Resource.ID = target.Name
	return operation
}

func attributesFromOperation(operation authz.Operation) Attributes {
	resources := make([]AttributeResource, 0, len(operation.Resource.Scope)+1)
	for _, resource := range operation.Resource.Scope {
		resources = append(resources, AttributeResource{Resource: resource.Type, Name: resource.ID})
	}
	if operation.Resource.Type != "" {
		resources = append(resources, AttributeResource{Resource: operation.Resource.Type, Name: operation.Resource.ID})
	}
	return Attributes{
		Service:   operation.Service,
		Method:    operation.Context[authorizationContextHTTPMethod],
		Action:    operation.Action,
		Resources: resources,
		Path:      operation.Context[authorizationContextHTTPPath],
	}
}

// AttributeExtractor derives authorization attributes from a request.
type AttributeExtractor func(r *http.Request) (*Attributes, error)

// PrefixedAttributesExtractor returns an extractor for paths below prefix.
func PrefixedAttributesExtractor(prefix string) AttributeExtractor {
	return func(r *http.Request) (*Attributes, error) {
		if !strings.HasPrefix(r.URL.Path, prefix) {
			return nil, nil
		}
		method, path := r.Method, strings.TrimPrefix(r.URL.Path, prefix)
		action, resources := DefaultRESTAttributeExtractor(method, path)
		return &Attributes{Method: method, Action: action, Resources: resources, Path: path}, nil
	}
}

// ServiceAttributesExtractor sets the target service on attributes returned by
// extractor.
func ServiceAttributesExtractor(service string, extractor AttributeExtractor) AttributeExtractor {
	return func(r *http.Request) (*Attributes, error) {
		attributes, err := extractor(r)
		if attributes != nil {
			attributes.Service = service
		}
		return attributes, err
	}
}

// plural
var MethodActionMapPlural = map[string]string{
	"GET":    "list",
	"POST":   "create",
	"DELETE": "removeBatch",
	"PUT":    "updateBatch",
	"PATCH":  "patchBatch",
}

// singular plural
var MethodActionMapSingular = map[string]string{
	"GET":    "get",
	"PUT":    "update",
	"DELETE": "remove",
	"PATCH":  "patch",
}

// DefaultRESTAttributeExtractor derives an action and resource path using the
// package's conventional REST method mapping.
func DefaultRESTAttributeExtractor(method string, path string) (string, []AttributeResource) {
	// example:
	// /api/v1/namespaces/default/pods/nginx-xxx -> ["namespaces", "default", "pods", "nginx-xxx"]
	// /api/v1/namespaces/default/pods -> ["namespaces", "default", "pods"]
	// /api/v1/namespaces/default -> ["namespaces", "default"]
	// /api/v1/namespaces -> ["namespaces"]
	// /api/v1 -> []
	resource, action := splitResourceAction(path)
	parts := removeEmpty(strings.Split(resource, "/"))
	if len(parts) == 0 {
		return action, nil
	}
	// if odd, it's a list request, e.g. GET /api/v1/namespaces/default/pods
	if len(parts)%2 != 0 {
		parts = append(parts, "")
		if action == "" {
			action = string(MethodActionMapPlural[method])
		}
	} else {
		if action == "" {
			action = string(MethodActionMapSingular[method])
		}
	}
	resources := []AttributeResource{}
	for i := 0; i < len(parts); i += 2 {
		resources = append(resources, AttributeResource{Resource: parts[i], Name: parts[i+1]})
	}
	return action, resources
}

func removeEmpty(arr []string) []string {
	w := 0
	for _, v := range arr {
		if v != "" {
			arr[w] = v
			w++
		}
	}
	return arr[:w]
}

// e.g. /zoos/{id}/animals/{name}:feed -> /zoos/{id}/animals/{name},feed
func splitResourceAction(path string) (string, string) {
	if i := strings.LastIndex(path, ":"); i < 0 {
		return path, ""
	} else {
		return path[:i], path[i+1:]
	}
}

// NewAttributeExtractionFilter installs extracted authorization attributes in
// the request context.
func NewAttributeExtractionFilter(extractor AttributeExtractor) Filter {
	return FilterFunc(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		attributes, err := extractor(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		ctx := WithAttributes(r.Context(), attributes)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// WithAttributes returns a context carrying authorization attributes.
func WithAttributes(ctx context.Context, attributes *Attributes) context.Context {
	return SetContextValue(ctx, "attributes", attributes)
}

// AttributesFromContext returns the authorization attributes carried by ctx.
func AttributesFromContext(ctx context.Context) *Attributes {
	return GetContextValue[*Attributes](ctx, "attributes")
}

// SetAttributeResourceName sets the final target resource name. Creation
// handlers use it after assigning a name so audit records contain that name.
func SetAttributeResourceName(ctx context.Context, name string) {
	attributes := AttributesFromContext(ctx)
	if len(attributes.Resources) == 0 {
		return
	}
	attributes.Resources[len(attributes.Resources)-1].Name = name
}
