package api

import (
	"context"
	"net/http"
	"strings"
)

type AttrbuteResource struct {
	Resource string `json:"resource,omitempty"`
	Name     string `json:"name,omitempty"`
}

type Attributes struct {
	Action    string             `json:"action,omitempty"`
	Resources []AttrbuteResource `json:"resources,omitempty"`
	Path      string             `json:"path,omitempty"`
}

type AttributeExtractor func(r *http.Request) (*Attributes, error)

func PrefixedAttributesExtractor(prefix string) AttributeExtractor {
	return func(r *http.Request) (*Attributes, error) {
		if !strings.HasPrefix(r.URL.Path, prefix) {
			return nil, nil
		}
		method, path := r.Method, strings.TrimPrefix(r.URL.Path, prefix)
		action, resources := DefaultRestAttributeExtractor(method, path)
		return &Attributes{Action: action, Resources: resources, Path: path}, nil
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

func DefaultRestAttributeExtractor(method string, path string) (string, []AttrbuteResource) {
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
	resources := []AttrbuteResource{}
	for i := 0; i < len(parts); i += 2 {
		resources = append(resources, AttrbuteResource{Resource: parts[i], Name: parts[i+1]})
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

func NewAttributeFilter(attributer AttributeExtractor) Filter {
	return FilterFunc(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		attributes, err := attributer(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		ctx := WithAttributes(r.Context(), attributes)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func WithAttributes(ctx context.Context, attributes *Attributes) context.Context {
	return SetContextValue(ctx, "attributes", attributes)
}

func AttributesFromContext(ctx context.Context) *Attributes {
	return GetContextValue[*Attributes](ctx, "attributes")
}

// InjectAttrName use to set name for current request attribute
// it usually called when a creation operation is successful
// it help audit to record what name this creation operation created
func InjectAttrName(ctx context.Context, name string) {
	attributes := AttributesFromContext(ctx)
	if len(attributes.Resources) == 0 {
		return
	}
	attributes.Resources[len(attributes.Resources)-1].Name = name
}
