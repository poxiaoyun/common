package authz

// ServiceID identifies one authorization service namespace.
type ServiceID string

// ResourceReference identifies one concrete resource by type and ID.
type ResourceReference struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// Scope is the authorization instance root when Path is empty, or a concrete
// root-to-resource path otherwise.
type Scope struct {
	Path []ResourceReference `json:"path,omitempty"`
}

// ResourceScope constructs a concrete root-to-resource path.
func ResourceScope(path ...ResourceReference) Scope {
	return Scope{Path: append([]ResourceReference(nil), path...)}
}
