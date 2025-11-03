package api

import (
	"net/http/httptest"
	"slices"
	"testing"

	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/meta"
)

func setUpWebhookAuthorization() (*WebhookAuthorizationProvider, func()) {
	provider := NewInMemoryAuthorizationProvider()
	serverhandler := NewWebhookAuthorizationServer(provider)
	handler := New().Group(serverhandler.Group()).Build()

	testserver := httptest.NewServer(handler)

	webhookProvider := NewWebhookAuthorizationProvider(&WebhookAuthorizationProviderOptions{
		WebhookOptions: WebhookOptions{Server: testserver.URL},
	})
	return webhookProvider, func() {
		testserver.Close()
	}
}

func TestWebhookAuthorization(t *testing.T) {
	webhookProvider, tearDown := setUpWebhookAuthorization()
	defer tearDown()

	t.Run("Test Organization Authorization", func(t *testing.T) {
		testOrganizationAuthorization(t, webhookProvider)
	})
	t.Run("Test Role Management", func(t *testing.T) {
		testRoleManagement(t, webhookProvider)
	})
	t.Run("Test Member Management", func(t *testing.T) {
		testMemberManagement(t, webhookProvider)
	})
}

func testOrganizationAuthorization(t *testing.T, provider AuthorizationProvider) {
	// add organizations
	org1 := &Organization{
		ObjectMetadata: meta.ObjectMetadata{ID: "org1", Name: "Organization 1"},
	}
	org2 := &Organization{
		ObjectMetadata: meta.ObjectMetadata{ID: "org2", Name: "Organization 2"},
	}
	if err := provider.CreateOrganization(t.Context(), org1); err != nil {
		t.Fatalf("failed to add organization: %v", err)
	}
	if err := provider.CreateOrganization(t.Context(), org2); err != nil {
		t.Fatalf("failed to add organization: %v", err)
	}

	// list organizations
	orgsPage, err := provider.ListOrganizations(t.Context(), ListOrganizationsOptions{})
	if err != nil {
		t.Fatalf("failed to list organizations: %v", err)
	}
	if len(orgsPage.Items) != 2 {
		t.Fatalf("expected 2 organizations, got %d", len(orgsPage.Items))
	}
	// limit size
	orgsPage, err = provider.ListOrganizations(t.Context(), ListOrganizationsOptions{ListOptions: meta.ListOptions{Size: 1}})
	if err != nil {
		t.Fatalf("failed to list organizations with size limit: %v", err)
	}
	if len(orgsPage.Items) != 1 {
		t.Fatalf("expected 1 organization, got %d", len(orgsPage.Items))
	}

	// get organization
	org, err := provider.GetOrganization(t.Context(), "org1")
	if err != nil {
		t.Fatalf("failed to get organization: %v", err)
	}
	if org.ID != "org1" {
		t.Fatalf("expected organization ID 'org1', got '%s'", org.ID)
	}

	// update organization
	org.Name = "Updated Organization 1"
	if err := provider.UpdateOrganization(t.Context(), org); err != nil {
		t.Fatalf("failed to update organization: %v", err)
	}
	updatedOrg, err := provider.GetOrganization(t.Context(), "org1")
	if err != nil {
		t.Fatalf("failed to get organization after update: %v", err)
	}
	if updatedOrg.Name != "Updated Organization 1" {
		t.Fatalf("expected updated organization name 'Updated Organization 1', got '%s'", updatedOrg.Name)
	}

	// exists organization
	exists, err := provider.ExistsOrganization(t.Context(), "org1")
	if err != nil {
		t.Fatalf("failed to check organization existence: %v", err)
	}
	if !exists {
		t.Fatalf("expected organization 'org1' to exist")
	}
	exists, err = provider.ExistsOrganization(t.Context(), "nonexistent")
	if err != nil {
		t.Fatalf("failed to check organization existence: %v", err)
	}
	if exists {
		t.Fatalf("expected organization 'nonexistent' to not exist")
	}

	// delete organization
	if err := provider.DeleteOrganization(t.Context(), "org2"); err != nil {
		t.Fatalf("failed to delete organization: %v", err)
	}
	exists, err = provider.ExistsOrganization(t.Context(), "org2")
	if err != nil {
		t.Fatalf("failed to check organization existence after deletion: %v", err)
	}
	if exists {
		t.Fatalf("expected organization 'org2' to not exist after deletion")
	}

	// delete non-existing organization
	if err := provider.DeleteOrganization(t.Context(), "nonexistent"); err != nil {
		if !errors.IsNotFound(err) {
			t.Fatalf("expected not found error when deleting non-existing organization, got: %v", err)
		}
	} else {
		t.Fatalf("expected error when deleting non-existing organization, got nil")
	}

	// remove organization
	if err := provider.DeleteOrganization(t.Context(), "org1"); err != nil {
		t.Fatalf("failed to delete organization: %v", err)
	}
}

func testRoleManagement(t *testing.T, provider AuthorizationProvider) {
	// setup organization
	org := &Organization{
		ObjectMetadata: meta.ObjectMetadata{ID: "org1", Name: "Organization 1"},
	}
	if err := provider.CreateOrganization(t.Context(), org); err != nil {
		if !errors.IsAlreadyExists(err) {
			t.Fatalf("failed to add organization: %v", err)
		}
	}

	// add roles
	adminRole := &Role{
		ObjectMetadata: meta.ObjectMetadata{ID: "admin"},
		Authorities: []Authority{
			{
				Service:   "*",
				Actions:   []string{"*"},
				Resources: []string{"**"},
			},
		},
	}
	developerRole := &Role{
		ObjectMetadata: meta.ObjectMetadata{ID: "developer"},
		Authorities: []Authority{
			{
				Service:   "compute",
				Actions:   []string{"get", "list", "create", "update", "delete"},
				Resources: []string{"namespaces:**", "pods:**", "services:**"},
			},
		},
	}
	if err := provider.CreateRole(t.Context(), "org1", adminRole); err != nil {
		t.Fatalf("failed to add role: %v", err)
	}
	if err := provider.CreateRole(t.Context(), "org1", developerRole); err != nil {
		t.Fatalf("failed to add role: %v", err)
	}

	// list roles
	rolesPage, err := provider.ListRoles(t.Context(), "org1", ListRolesOptions{})
	if err != nil {
		t.Fatalf("failed to list roles: %v", err)
	}
	if len(rolesPage.Items) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(rolesPage.Items))
	}

	// get role
	role, err := provider.GetRole(t.Context(), "org1", "admin")
	if err != nil {
		t.Fatalf("failed to get role: %v", err)
	}
	if role.ID != "admin" {
		t.Fatalf("expected role ID 'admin', got '%s'", role.ID)
	}

	// update role
	role.Authorities = append(role.Authorities, Authority{
		Service:   "storage",
		Actions:   []string{"get", "list"},
		Resources: []string{"buckets:**"},
	})
	if err := provider.UpdateRole(t.Context(), "org1", role); err != nil {
		t.Fatalf("failed to update role: %v", err)
	}
	updatedRole, err := provider.GetRole(t.Context(), "org1", "admin")
	if err != nil {
		t.Fatalf("failed to get role after update: %v", err)
	}
	if len(updatedRole.Authorities) != 2 {
		t.Fatalf("expected updated role to have 2 authorities, got %d", len(updatedRole.Authorities))
	}

	// delete role
	if err := provider.DeleteRole(t.Context(), "org1", "developer"); err != nil {
		t.Fatalf("failed to delete role: %v", err)
	}
	_, err = provider.GetRole(t.Context(), "org1", "developer")
	if err == nil {
		t.Fatalf("expected error when getting deleted role, got nil")
	}

	// list roles after deletion
	rolesPage, err = provider.ListRoles(t.Context(), "org1", ListRolesOptions{})
	if err != nil {
		t.Fatalf("failed to list roles after deletion: %v", err)
	}
	if len(rolesPage.Items) != 1 {
		t.Fatalf("expected 1 role after deletion, got %d", len(rolesPage.Items))
	}

	// delete
	if err := provider.DeleteRole(t.Context(), "org1", "admin"); err != nil {
		t.Fatalf("failed to delete role: %v", err)
	}
}

func testMemberManagement(t *testing.T, provider AuthorizationProvider) {
	// setup organization
	org := &Organization{
		ObjectMetadata: meta.ObjectMetadata{ID: "org1", Name: "Organization 1"},
	}
	if err := provider.CreateOrganization(t.Context(), org); err != nil {
		if !errors.IsAlreadyExists(err) {
			t.Fatalf("failed to add organization: %v", err)
		}
	}

	// add member
	member := &Member{
		ObjectMetadata: meta.ObjectMetadata{ID: "user1"},
		Roles:          []string{"admin", "developer"},
	}
	if err := provider.AddMember(t.Context(), "org1", member); err != nil {
		t.Fatalf("failed to add member: %v", err)
	}

	// get member
	gotMember, err := provider.GetMember(t.Context(), "org1", "user1")
	if err != nil {
		t.Fatalf("failed to get member: %v", err)
	}
	if gotMember.ID != "user1" {
		t.Fatalf("expected member ID 'user1', got '%s'", gotMember.ID)
	}

	// update member
	gotMember.Roles = []string{"admin", "tester"}
	if err := provider.UpdateMember(t.Context(), "org1", gotMember); err != nil {
		t.Fatalf("failed to update member: %v", err)
	}
	updatedMember, err := provider.GetMember(t.Context(), "org1", "user1")
	if err != nil {
		t.Fatalf("failed to get member after update: %v", err)
	}
	if len(updatedMember.Roles) != 2 || updatedMember.Roles[1] != "tester" {
		t.Fatalf("expected updated member roles to include 'tester', got %v", updatedMember.Roles)
	}

	// list members
	membersPage, err := provider.ListMembers(t.Context(), "org1", ListMembersOptions{})
	if err != nil {
		t.Fatalf("failed to list members: %v", err)
	}
	if len(membersPage.Items) != 1 {
		t.Fatalf("expected 1 member, got %d", len(membersPage.Items))
	}
	// create role for revoke test
	testRole := &Role{
		ObjectMetadata: meta.ObjectMetadata{ID: "admin"},
		Authorities: []Authority{
			{
				Service:   "*",
				Actions:   []string{"*"},
				Resources: []string{"**"},
			},
		},
	}
	if err := provider.CreateRole(t.Context(), "org1", testRole); err != nil {
		t.Fatalf("failed to create role for revoke test: %v", err)
	}
	viewerRole := &Role{
		ObjectMetadata: meta.ObjectMetadata{ID: "viewer"},
		Authorities: []Authority{
			{
				Service:   "compute",
				Actions:   []string{"get", "list"},
				Resources: []string{"namespaces:**", "pods:**", "services:**"},
			},
		},
	}
	if err := provider.CreateRole(t.Context(), "org1", viewerRole); err != nil {
		t.Fatalf("failed to create viewer role for assign test: %v", err)
	}

	// assign role
	if err := provider.AssignRole(t.Context(), "org1", "user1", "viewer"); err != nil {
		t.Fatalf("failed to assign role to member: %v", err)
	}
	memberWithNewRole, err := provider.GetMember(t.Context(), "org1", "user1")
	if err != nil {
		t.Fatalf("failed to get member after role assignment: %v", err)
	}
	found := slices.Contains(memberWithNewRole.Roles, "viewer")
	if !found {
		t.Fatalf("expected member roles to include 'viewer', got %v", memberWithNewRole.Roles)
	}

	// revoke role
	if err := provider.RevokeRole(t.Context(), "org1", "user1", "admin"); err != nil {
		t.Fatalf("failed to revoke role from member: %v", err)
	}
	memberAfterRevoke, err := provider.GetMember(t.Context(), "org1", "user1")
	if err != nil {
		t.Fatalf("failed to get member after role revocation: %v", err)
	}
	found = slices.Contains(memberAfterRevoke.Roles, "admin")
	if found {
		t.Fatalf("expected member roles to not include 'admin', got %v", memberAfterRevoke.Roles)
	}

	// user organizations
	userOrgsPage, err := provider.UserOrganizations(t.Context(), "user1", ListUserOrganizationsOptions{})
	if err != nil {
		t.Fatalf("failed to list user organizations: %v", err)
	}
	if len(userOrgsPage.Items) == 0 {
		t.Fatalf("expected user to be a member of at least one organization")
	}
	if slices.ContainsFunc(userOrgsPage.Items, func(org OrganizationRoles) bool { return org.ID == "org1" }) == false {
		t.Fatalf("expected user to be a member of organization 'org1'")
	}

	// delete member
	if err := provider.DeleteMember(t.Context(), "org1", "user1"); err != nil {
		t.Fatalf("failed to delete member: %v", err)
	}
	_, err = provider.GetMember(t.Context(), "org1", "user1")
	if err == nil {
		t.Fatalf("expected error when getting deleted member, got nil")
	}

	// list members after deletion
	membersPage, err = provider.ListMembers(t.Context(), "org1", ListMembersOptions{})
	if err != nil {
		t.Fatalf("failed to list members after deletion: %v", err)
	}
	if len(membersPage.Items) != 0 {
		t.Fatalf("expected 0 members after deletion, got %d", len(membersPage.Items))
	}
}
