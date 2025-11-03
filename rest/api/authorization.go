package api

import (
	"context"

	"xiaoshiai.cn/common/meta"
)

type Role struct {
	meta.ObjectMetadata `json:",inline"`
	// Hidden indicates whether this role is hidden
	// hidden role will not be listed in role lists
	Hidden bool `json:"hidden,omitempty"`
	// IsSystem indicates whether this is a system role
	// System roles are created by the system and cannot be modified or deleted
	IsSystem bool `json:"isSystem,omitempty"`
	// Authorities is the list of authorities granted to this role
	Authorities []Authority `json:"authorities,omitempty"`
}

type Organization struct {
	meta.ObjectMetadata `json:",inline"`
}

type Member struct {
	meta.ObjectMetadata `json:",inline"`
	Organization        string `json:"organization,omitempty"`
	// Roles is the list of roles assigned to this member
	Roles []string `json:"roles,omitempty"`
}

type OrganizationRoles struct {
	Organization `json:",inline"`
	Roles        []string `json:"roles,omitempty"`
}

type ListOrganizationsOptions struct {
	meta.ListOptions
}

type ListMembersOptions struct {
	meta.ListOptions
}

type ListRolesOptions struct {
	meta.ListOptions
}

type ListUserOrganizationsOptions struct {
	meta.ListOptions
}

// AuthorizationProvider is the interface for authorization management
// It provides methods for managing roles, organizations and member's roles.
// Callers must ensure the user is authenticated before calling these methods.
// Implementations must check permissions when performing operations.
// [AuthenticateFromContext] can be used to get the authenticated user from context
type AuthorizationProvider interface {
	// AuthorizationProvider also implements the [Authorizer] interface
	Authorizer

	OrganizationProvider

	ListMembers(ctx context.Context, org string, options ListMembersOptions) (Page[Member], error)
	GetMember(ctx context.Context, org, member string) (*Member, error)
	AddMember(ctx context.Context, org string, member *Member) error
	UpdateMember(ctx context.Context, org string, member *Member) error
	DeleteMember(ctx context.Context, org, member string) error

	// UserOrganizations lists organizations that the user is a member of
	UserOrganizations(ctx context.Context, user string, options ListUserOrganizationsOptions) (Page[OrganizationRoles], error)

	ListRoles(ctx context.Context, org string, options ListRolesOptions) (Page[Role], error)
	GetRole(ctx context.Context, org, role string) (*Role, error)
	CreateRole(ctx context.Context, org string, role *Role) error
	UpdateRole(ctx context.Context, org string, role *Role) error
	DeleteRole(ctx context.Context, org, role string) error

	AssignRole(ctx context.Context, org, member, role string) error
	RevokeRole(ctx context.Context, org, member, role string) error
}

// OrganizationProvider is the interface for organization management
type OrganizationProvider interface {
	ListOrganizations(ctx context.Context, options ListOrganizationsOptions) (Page[Organization], error)
	GetOrganization(ctx context.Context, org string) (*Organization, error)
	ExistsOrganization(ctx context.Context, org string) (bool, error)
	CreateOrganization(ctx context.Context, org *Organization) error
	UpdateOrganization(ctx context.Context, org *Organization) error
	DeleteOrganization(ctx context.Context, org string) error
}
