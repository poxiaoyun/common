package api

import (
	"context"
	"slices"
	"sort"
	"sync"

	"xiaoshiai.cn/common/errors"
)

func NewInMemoryAuthorizationProvider() *InMemoryAuthorizationProvider {
	return &InMemoryAuthorizationProvider{
		organizations: make(map[string]*Organization),
		roles:         make(map[string]map[string]*Role),
		members:       make(map[string]map[string]*Member),
	}
}

var _ AuthorizationProvider = &InMemoryAuthorizationProvider{}

type InMemoryAuthorizationProvider struct {
	lock          sync.RWMutex
	organizations map[string]*Organization
	roles         map[string]map[string]*Role   // org -> roleName -> Role
	members       map[string]map[string]*Member // org -> memberName -> Member
}

// AddMember implements AuthorizationProvider.
func (i *InMemoryAuthorizationProvider) AddMember(ctx context.Context, org string, member *Member) error {
	i.lock.Lock()
	defer i.lock.Unlock()
	if _, ok := i.organizations[org]; !ok {
		return errors.NewNotFound("organization", org)
	}
	if i.members[org] == nil {
		i.members[org] = make(map[string]*Member)
	}
	if _, ok := i.members[org][member.ID]; ok {
		return errors.NewAlreadyExists("member", member.ID)
	}
	shadow := *member
	i.members[org][member.ID] = &shadow
	return nil
}

// AssignRole implements AuthorizationProvider.
func (i *InMemoryAuthorizationProvider) AssignRole(ctx context.Context, org string, member string, role string) error {
	i.lock.Lock()
	defer i.lock.Unlock()
	if _, ok := i.organizations[org]; !ok {
		return errors.NewNotFound("organization", org)
	}
	rolesMap := i.roles[org]
	if rolesMap == nil {
		return errors.NewNotFound("role", role)
	}
	if _, ok := rolesMap[role]; !ok {
		return errors.NewNotFound("role", role)
	}
	if i.members[org] == nil {
		return errors.NewNotFound("member", member)
	}
	m, ok := i.members[org][member]
	if !ok {
		return errors.NewNotFound("member", member)
	}
	// ensure no duplicate
	if slices.Contains(m.Roles, role) {
		return nil
	}
	m.Roles = append(m.Roles, role)
	return nil
}

// Authorize implements AuthorizationProvider.
func (i *InMemoryAuthorizationProvider) Authorize(ctx context.Context, user UserInfo, a Attributes) (authorized Decision, reason string, err error) {
	i.lock.RLock()
	defer i.lock.RUnlock()
	if user.ID == "" {
		return DecisionDeny, "anonymous user", nil
	}

	// iterate orgs and roles
	for org := range i.organizations {
		membersMap := i.members[org]
		if membersMap == nil {
			continue
		}
		m, ok := membersMap[user.ID]
		if !ok {
			continue
		}
		for _, roleName := range m.Roles {
			roleMap := i.roles[org]
			if roleMap == nil {
				continue
			}
			r, ok := roleMap[roleName]
			if !ok || r == nil {
				continue
			}
			// check authorities
			for _, auth := range r.Authorities {
				if auth.MatchAttributes(a) {
					return DecisionAllow, "", nil
				}
			}
		}
	}
	return DecisionNoOpinion, "", nil
}

// CreateOrganization implements AuthorizationProvider.
func (i *InMemoryAuthorizationProvider) CreateOrganization(ctx context.Context, org *Organization) error {
	i.lock.Lock()
	defer i.lock.Unlock()
	if _, ok := i.organizations[org.ID]; ok {
		return errors.NewAlreadyExists("organization", org.ID)
	}
	o := *org
	i.organizations[o.ID] = &o
	return nil
}

// CreateRole implements AuthorizationProvider.
func (i *InMemoryAuthorizationProvider) CreateRole(ctx context.Context, org string, role *Role) error {
	i.lock.Lock()
	defer i.lock.Unlock()
	if _, ok := i.organizations[org]; !ok {
		return errors.NewNotFound("organization", org)
	}
	if i.roles[org] == nil {
		i.roles[org] = make(map[string]*Role)
	}
	if _, ok := i.roles[org][role.ID]; ok {
		return errors.NewAlreadyExists("role", role.ID)
	}
	r := *role
	i.roles[org][r.ID] = &r
	return nil
}

// DeleteMember implements AuthorizationProvider.
func (i *InMemoryAuthorizationProvider) DeleteMember(ctx context.Context, org string, member string) error {
	i.lock.Lock()
	defer i.lock.Unlock()
	if i.members[org] == nil {
		return errors.NewNotFound("member", member)
	}
	if _, ok := i.members[org][member]; !ok {
		return errors.NewNotFound("member", member)
	}
	delete(i.members[org], member)
	return nil
}

// DeleteOrganization implements AuthorizationProvider.
func (i *InMemoryAuthorizationProvider) DeleteOrganization(ctx context.Context, org string) error {
	i.lock.Lock()
	defer i.lock.Unlock()
	if _, ok := i.organizations[org]; !ok {
		return errors.NewNotFound("organization", org)
	}
	delete(i.organizations, org)
	delete(i.roles, org)
	delete(i.members, org)
	return nil
}

// DeleteRole implements AuthorizationProvider.
func (i *InMemoryAuthorizationProvider) DeleteRole(ctx context.Context, org string, role string) error {
	i.lock.Lock()
	defer i.lock.Unlock()
	if i.roles[org] == nil {
		return errors.NewNotFound("role", role)
	}
	if _, ok := i.roles[org][role]; !ok {
		return errors.NewNotFound("role", role)
	}
	delete(i.roles[org], role)
	// remove role from members
	if mems := i.members[org]; mems != nil {
		for _, m := range mems {
			newRoles := []string{}
			for _, r := range m.Roles {
				if r != role {
					newRoles = append(newRoles, r)
				}
			}
			m.Roles = newRoles
		}
	}
	return nil
}

// ExistsOrganization implements AuthorizationProvider.
func (i *InMemoryAuthorizationProvider) ExistsOrganization(ctx context.Context, org string) (bool, error) {
	i.lock.RLock()
	defer i.lock.RUnlock()
	_, ok := i.organizations[org]
	return ok, nil
}

// GetMember implements AuthorizationProvider.
func (i *InMemoryAuthorizationProvider) GetMember(ctx context.Context, org string, member string) (*Member, error) {
	i.lock.RLock()
	defer i.lock.RUnlock()
	if i.members[org] == nil {
		return nil, errors.NewNotFound("member", member)
	}
	m, ok := i.members[org][member]
	if !ok {
		return nil, errors.NewNotFound("member", member)
	}
	mm := *m
	return &mm, nil
}

// GetOrganization implements AuthorizationProvider.
func (i *InMemoryAuthorizationProvider) GetOrganization(ctx context.Context, org string) (*Organization, error) {
	i.lock.RLock()
	defer i.lock.RUnlock()
	o, ok := i.organizations[org]
	if !ok {
		return nil, errors.NewNotFound("organization", org)
	}
	oo := *o
	return &oo, nil
}

// GetRole implements AuthorizationProvider.
func (i *InMemoryAuthorizationProvider) GetRole(ctx context.Context, org string, role string) (*Role, error) {
	i.lock.RLock()
	defer i.lock.RUnlock()
	if i.roles[org] == nil {
		return nil, errors.NewNotFound("role", role)
	}
	r, ok := i.roles[org][role]
	if !ok {
		return nil, errors.NewNotFound("role", role)
	}
	rr := *r
	return &rr, nil
}

// ListMembers implements AuthorizationProvider.
func (i *InMemoryAuthorizationProvider) ListMembers(ctx context.Context, org string, options ListMembersOptions) (Page[Member], error) {
	i.lock.RLock()
	defer i.lock.RUnlock()
	list := []Member{}
	if i.members[org] != nil {
		for _, m := range i.members[org] {
			list = append(list, *m)
		}
	}
	// stable order by name
	sort.Slice(list, func(a, b int) bool { return list[a].ID < list[b].ID })
	return PageFromListOptions(list, options.ListOptions, func(item Member) string { return item.ID }, nil), nil
}

// ListOrganizations implements AuthorizationProvider.
func (i *InMemoryAuthorizationProvider) ListOrganizations(ctx context.Context, options ListOrganizationsOptions) (Page[Organization], error) {
	i.lock.RLock()
	defer i.lock.RUnlock()
	list := []Organization{}
	for _, o := range i.organizations {
		list = append(list, *o)
	}
	sort.Slice(list, func(a, b int) bool { return list[a].ID < list[b].ID })
	return PageFromListOptions(list, options.ListOptions, func(item Organization) string { return item.ID }, nil), nil
}

// ListRoles implements AuthorizationProvider.
func (i *InMemoryAuthorizationProvider) ListRoles(ctx context.Context, org string, options ListRolesOptions) (Page[Role], error) {
	i.lock.RLock()
	defer i.lock.RUnlock()
	list := []Role{}
	if i.roles[org] != nil {
		for _, r := range i.roles[org] {
			list = append(list, *r)
		}
	}
	sort.Slice(list, func(a, b int) bool { return list[a].ID < list[b].ID })
	return PageFromListOptions(list, options.ListOptions, func(item Role) string { return item.ID }, nil), nil
}

// RevokeRole implements AuthorizationProvider.
func (i *InMemoryAuthorizationProvider) RevokeRole(ctx context.Context, org string, member string, role string) error {
	i.lock.Lock()
	defer i.lock.Unlock()
	if i.members[org] == nil {
		return errors.NewNotFound("member", member)
	}
	m, ok := i.members[org][member]
	if !ok {
		return errors.NewNotFound("member", member)
	}
	newRoles := []string{}
	for _, r := range m.Roles {
		if r != role {
			newRoles = append(newRoles, r)
		}
	}
	m.Roles = newRoles
	return nil
}

// UpdateMember implements AuthorizationProvider.
func (i *InMemoryAuthorizationProvider) UpdateMember(ctx context.Context, org string, member *Member) error {
	i.lock.Lock()
	defer i.lock.Unlock()
	if i.members[org] == nil {
		return errors.NewNotFound("member", member.ID)
	}
	if _, ok := i.members[org][member.ID]; !ok {
		return errors.NewNotFound("member", member.ID)
	}
	m := *member
	i.members[org][m.ID] = &m
	return nil
}

// UpdateOrganization implements AuthorizationProvider.
func (i *InMemoryAuthorizationProvider) UpdateOrganization(ctx context.Context, org *Organization) error {
	i.lock.Lock()
	defer i.lock.Unlock()
	if _, ok := i.organizations[org.ID]; !ok {
		return errors.NewNotFound("organization", org.ID)
	}
	o := *org
	i.organizations[o.ID] = &o
	return nil
}

// UpdateRole implements AuthorizationProvider.
func (i *InMemoryAuthorizationProvider) UpdateRole(ctx context.Context, org string, role *Role) error {
	i.lock.Lock()
	defer i.lock.Unlock()
	if i.roles[org] == nil {
		return errors.NewNotFound("role", role.ID)
	}
	if _, ok := i.roles[org][role.ID]; !ok {
		return errors.NewNotFound("role", role.ID)
	}
	r := *role
	i.roles[org][r.ID] = &r
	return nil
}

// UserOrganizations implements AuthorizationProvider.
func (i *InMemoryAuthorizationProvider) UserOrganizations(ctx context.Context, user string, options ListUserOrganizationsOptions) (Page[OrganizationRoles], error) {
	i.lock.RLock()
	defer i.lock.RUnlock()
	list := []OrganizationRoles{}
	for orgName, membersMap := range i.members {
		if membersMap == nil {
			continue
		}
		m, ok := membersMap[user]
		if !ok {
			continue
		}
		orgObj, ok := i.organizations[orgName]
		if !ok {
			continue
		}
		orgRoles := OrganizationRoles{
			Organization: *orgObj,
			Roles:        m.Roles,
		}
		list = append(list, orgRoles)
	}
	// stable order by name
	sort.Slice(list, func(a, b int) bool { return list[a].ID < list[b].ID })
	return PageFromListOptions(list, options.ListOptions, func(item OrganizationRoles) string { return item.ID }, nil), nil
}
