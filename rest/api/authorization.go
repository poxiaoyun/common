package api

import (
	"context"

	"xiaoshiai.cn/common/meta"
)

type Metadata struct {
	ID                string            `json:"id,omitempty"`
	Name              string            `json:"name,omitempty"`
	CreationTimestamp meta.Time         `json:"creationTimestamp,omitempty"`
	DeletionTimestamp *meta.Time        `json:"deletionTimestamp,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	Description       string            `json:"description,omitempty"`
}

type Role struct {
	Metadata    `json:",inline"`
	Hidden      bool        `json:"hidden,omitempty"` // hidden role will not be listed
	Authorities []Authority `json:"authorities,omitempty"`
}

type ListOrganizationsOptions struct {
	ListOptions
	Page         int
	Size         int
	Search       string
	SearchFields []string
	// Sort is the sort order of the list.  The format is a comma separated list of fields, optionally
	// prefixed by "+" or "-".  The default is "+metadata.name", which sorts by the object's name.
	// For example, "-metadata.name,metadata.creationTimestamp" sorts first by descending name, and then by
	// ascending creation timestamp.
	// name is alias for metadata.name
	// time is alias for metadata.creationTimestamp
	Sort string
}

type Organization struct {
	Metadata `json:",inline"`
}

type AuthorizationProvider interface {
	ListOrganizations(ctx context.Context, options ListOrganizationsOptions) (Page[Organization], error)
}
