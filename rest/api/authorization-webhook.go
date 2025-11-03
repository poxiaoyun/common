package api

import (
	"context"
	stderrors "errors"
	"fmt"
	"net/http"

	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/httpclient"
)

type WebhookAuthorizationProviderOptions struct {
	WebhookOptions `json:",inline"`
}

func NewWebhookAuthorizationProvider(options *WebhookAuthorizationProviderOptions) *WebhookAuthorizationProvider {
	cli, err := NewHttpClientFromWebhookOptions(&options.WebhookOptions)
	if err != nil {
		return nil
	}
	cli.OnRequest = func(req *http.Request) error {
		return nil
	}
	return &WebhookAuthorizationProvider{cli: cli}
}

var _ AuthorizationProvider = &WebhookAuthorizationProvider{}

type WebhookAuthorizationProvider struct {
	cli *httpclient.Client
}

// Authorize implements AuthorizationProvider.
func (w *WebhookAuthorizationProvider) Authorize(ctx context.Context, user UserInfo, a Attributes) (authorized Decision, reason string, err error) {
	resp := &WebhookAuthorizationResponse{}
	if err := w.cli.Post("/authorize").JSON(WebhookAuthorizationRequest{UserInfo: user, Attributes: a}).Return(resp).Send(ctx); err != nil {
		return DecisionNoOpinion, "", err
	}
	if resp.Error != "" {
		return DecisionNoOpinion, resp.Reason, stderrors.New(resp.Error)
	}
	return resp.Decision, resp.Reason, nil
}

// ListOrganizations implements AuthorizationProvider.
func (w *WebhookAuthorizationProvider) ListOrganizations(ctx context.Context, options ListOrganizationsOptions) (Page[Organization], error) {
	page := Page[Organization]{}
	queries := httpclient.ListOptionsToQuery(options.ListOptions)
	if err := w.cli.Get("/organizations").Queries(queries).Return(&page).Send(ctx); err != nil {
		return Page[Organization]{}, err
	}
	return page, nil
}

// GetOrganization implements AuthorizationProvider.
func (w *WebhookAuthorizationProvider) GetOrganization(ctx context.Context, org string) (*Organization, error) {
	organization := &Organization{}
	if err := w.cli.Get(fmt.Sprintf("/organizations/%s", org)).Return(organization).Send(ctx); err != nil {
		return nil, err
	}
	return organization, nil
}

// CreateOrganization implements AuthorizationProvider.
func (w *WebhookAuthorizationProvider) CreateOrganization(ctx context.Context, org *Organization) error {
	return w.cli.Post("/organizations").JSON(org).Send(ctx)
}

// UpdateOrganization implements AuthorizationProvider.
func (w *WebhookAuthorizationProvider) UpdateOrganization(ctx context.Context, org *Organization) error {
	return w.cli.Put(fmt.Sprintf("/organizations/%s", org.ID)).JSON(org).Send(ctx)
}

// DeleteOrganization implements AuthorizationProvider.
func (w *WebhookAuthorizationProvider) DeleteOrganization(ctx context.Context, org string) error {
	return w.cli.Delete(fmt.Sprintf("/organizations/%s", org)).Send(ctx)
}

// ExistsOrganization implements AuthorizationProvider.
func (w *WebhookAuthorizationProvider) ExistsOrganization(ctx context.Context, org string) (bool, error) {
	resp, err := w.cli.Head(fmt.Sprintf("/organizations/%s", org)).Do(ctx)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return resp.StatusCode == http.StatusOK, nil
}

// UserOrganizations implements AuthorizationProvider.
func (w *WebhookAuthorizationProvider) UserOrganizations(ctx context.Context, user string, options ListUserOrganizationsOptions) (Page[OrganizationRoles], error) {
	page := Page[OrganizationRoles]{}
	queries := httpclient.ListOptionsToQuery(options.ListOptions)
	if err := w.cli.Get(fmt.Sprintf("/users/%s/organizations", user)).Queries(queries).Return(&page).Send(ctx); err != nil {
		return Page[OrganizationRoles]{}, err
	}
	return page, nil
}

// ListRoles implements AuthorizationProvider.
func (w *WebhookAuthorizationProvider) ListRoles(ctx context.Context, org string, options ListRolesOptions) (Page[Role], error) {
	page := Page[Role]{}
	queries := httpclient.ListOptionsToQuery(options.ListOptions)
	if err := w.cli.Get(fmt.Sprintf("/organizations/%s/roles", org)).Queries(queries).Return(&page).Send(ctx); err != nil {
		return Page[Role]{}, err
	}
	return page, nil
}

// CreateRole implements AuthorizationProvider.
func (w *WebhookAuthorizationProvider) CreateRole(ctx context.Context, org string, role *Role) error {
	return w.cli.Post(fmt.Sprintf("/organizations/%s/roles", org)).JSON(role).Send(ctx)
}

// GetRole implements AuthorizationProvider.
func (w *WebhookAuthorizationProvider) GetRole(ctx context.Context, org string, role string) (*Role, error) {
	r := &Role{}
	if err := w.cli.Get(fmt.Sprintf("/organizations/%s/roles/%s", org, role)).Return(r).Send(ctx); err != nil {
		return nil, err
	}
	return r, nil
}

// DeleteRole implements AuthorizationProvider.
func (w *WebhookAuthorizationProvider) DeleteRole(ctx context.Context, org string, role string) error {
	return w.cli.Delete(fmt.Sprintf("/organizations/%s/roles/%s", org, role)).Send(ctx)
}

// UpdateRole implements AuthorizationProvider.
func (w *WebhookAuthorizationProvider) UpdateRole(ctx context.Context, org string, role *Role) error {
	return w.cli.Put(fmt.Sprintf("/organizations/%s/roles/%s", org, role.ID)).JSON(role).Send(ctx)
}

// ListMembers implements AuthorizationProvider.
func (w *WebhookAuthorizationProvider) ListMembers(ctx context.Context, org string, options ListMembersOptions) (Page[Member], error) {
	page := Page[Member]{}
	queries := httpclient.ListOptionsToQuery(options.ListOptions)
	if err := w.cli.Get(fmt.Sprintf("/organizations/%s/members", org)).Queries(queries).Return(&page).Send(ctx); err != nil {
		return Page[Member]{}, err
	}
	return page, nil
}

// GetMember implements AuthorizationProvider.
func (w *WebhookAuthorizationProvider) GetMember(ctx context.Context, org string, member string) (*Member, error) {
	mem := &Member{}
	if err := w.cli.Get(fmt.Sprintf("/organizations/%s/members/%s", org, member)).Return(mem).Send(ctx); err != nil {
		return nil, err
	}
	return mem, nil
}

// AddMember implements AuthorizationProvider.
func (w *WebhookAuthorizationProvider) AddMember(ctx context.Context, org string, member *Member) error {
	return w.cli.Post(fmt.Sprintf("/organizations/%s/members", org)).JSON(member).Send(ctx)
}

// UpdateMember implements AuthorizationProvider.
func (w *WebhookAuthorizationProvider) UpdateMember(ctx context.Context, org string, member *Member) error {
	return w.cli.Put(fmt.Sprintf("/organizations/%s/members/%s", org, member.ID)).JSON(member).Send(ctx)
}

// DeleteMember implements AuthorizationProvider.
func (w *WebhookAuthorizationProvider) DeleteMember(ctx context.Context, org string, member string) error {
	return w.cli.Delete(fmt.Sprintf("/organizations/%s/members/%s", org, member)).Send(ctx)
}

// AssignRole implements AuthorizationProvider.
func (w *WebhookAuthorizationProvider) AssignRole(ctx context.Context, org string, member string, role string) error {
	return w.cli.Put(fmt.Sprintf("/organizations/%s/members/%s/roles/%s", org, member, role)).Send(ctx)
}

// RevokeRole implements AuthorizationProvider.
func (w *WebhookAuthorizationProvider) RevokeRole(ctx context.Context, org string, member string, role string) error {
	return w.cli.Delete(fmt.Sprintf("/organizations/%s/members/%s/roles/%s", org, member, role)).Send(ctx)
}

// WebhookAuthorizationServer is the server of WebhookAuthorizationProvider
// it wraps the provider to serve HTTP requests
type WebhookAuthorizationServer struct {
	Provider AuthorizationProvider
}

func NewWebhookAuthorizationServer(provider AuthorizationProvider) *WebhookAuthorizationServer {
	return &WebhookAuthorizationServer{Provider: provider}
}

func (s *WebhookAuthorizationServer) Authorize(w http.ResponseWriter, r *http.Request) {
	On(w, r, func(ctx context.Context) (any, error) {
		req := WebhookAuthorizationRequest{}
		if err := Body(r, &req); err != nil {
			return nil, err
		}
		decision, reason, err := s.Provider.Authorize(ctx, req.UserInfo, req.Attributes)
		if err != nil {
			return &WebhookAuthorizationResponse{Decision: decision, Reason: reason, Error: err.Error()}, nil
		}
		return &WebhookAuthorizationResponse{Decision: decision, Reason: reason}, nil
	})
}

func (s *WebhookAuthorizationServer) ListOrganizations(w http.ResponseWriter, r *http.Request) {
	On(w, r, func(ctx context.Context) (any, error) {
		return s.Provider.ListOrganizations(ctx, ListOrganizationsOptions{ListOptions: GetListOptions(r)})
	})
}

func (s *WebhookAuthorizationServer) GetOrganization(w http.ResponseWriter, r *http.Request) {
	s.onOrg(w, r, func(ctx context.Context, org string) (any, error) {
		return s.Provider.GetOrganization(ctx, org)
	})
}

func (s *WebhookAuthorizationServer) CreateOrganization(w http.ResponseWriter, r *http.Request) {
	On(w, r, func(ctx context.Context) (any, error) {
		org := &Organization{}
		if err := Body(r, org); err != nil {
			return nil, err
		}
		return nil, s.Provider.CreateOrganization(ctx, org)
	})
}

func (s *WebhookAuthorizationServer) UpdateOrganization(w http.ResponseWriter, r *http.Request) {
	s.onOrg(w, r, func(ctx context.Context, org string) (any, error) {
		o := &Organization{}
		if err := Body(r, o); err != nil {
			return nil, err
		}
		return nil, s.Provider.UpdateOrganization(ctx, o)
	})
}

func (s *WebhookAuthorizationServer) DeleteOrganization(w http.ResponseWriter, r *http.Request) {
	s.onOrg(w, r, func(ctx context.Context, org string) (any, error) {
		return nil, s.Provider.DeleteOrganization(ctx, org)
	})
}

func (s *WebhookAuthorizationServer) ExistsOrganization(w http.ResponseWriter, r *http.Request) {
	s.onOrg(w, r, func(ctx context.Context, org string) (any, error) {
		exists, err := s.Provider.ExistsOrganization(ctx, org)
		if err != nil {
			return nil, err
		}
		if !exists {
			return ResponseStatusOnly(http.StatusNotFound), nil
		}
		return Empty, nil
	})
}

func (s *WebhookAuthorizationServer) UserOrganizations(w http.ResponseWriter, r *http.Request) {
	On(w, r, func(ctx context.Context) (any, error) {
		user := Path(r, "user", "")
		if user == "" {
			return nil, errors.NewBadRequest("user is required")
		}
		return s.Provider.UserOrganizations(ctx, user, ListUserOrganizationsOptions{ListOptions: GetListOptions(r)})
	})
}

func (s *WebhookAuthorizationServer) ListRoles(w http.ResponseWriter, r *http.Request) {
	s.onOrg(w, r, func(ctx context.Context, org string) (any, error) {
		return s.Provider.ListRoles(ctx, org, ListRolesOptions{ListOptions: GetListOptions(r)})
	})
}

func (s *WebhookAuthorizationServer) CreateRole(w http.ResponseWriter, r *http.Request) {
	s.onOrg(w, r, func(ctx context.Context, org string) (any, error) {
		role := &Role{}
		if err := Body(r, role); err != nil {
			return nil, err
		}
		return nil, s.Provider.CreateRole(ctx, org, role)
	})
}

func (s *WebhookAuthorizationServer) GetRole(w http.ResponseWriter, r *http.Request) {
	s.onOrg(w, r, func(ctx context.Context, org string) (any, error) {
		role := Path(r, "role", "")
		if role == "" {
			return nil, errors.NewBadRequest("role is required")
		}
		return s.Provider.GetRole(ctx, org, role)
	})
}

func (s *WebhookAuthorizationServer) DeleteRole(w http.ResponseWriter, r *http.Request) {
	s.onOrg(w, r, func(ctx context.Context, org string) (any, error) {
		role := Path(r, "role", "")
		if role == "" {
			return nil, errors.NewBadRequest("role is required")
		}
		return nil, s.Provider.DeleteRole(ctx, org, role)
	})
}

func (s *WebhookAuthorizationServer) UpdateRole(w http.ResponseWriter, r *http.Request) {
	s.onOrg(w, r, func(ctx context.Context, org string) (any, error) {
		role := &Role{}
		if err := Body(r, role); err != nil {
			return nil, err
		}
		return nil, s.Provider.UpdateRole(ctx, org, role)
	})
}

func (s *WebhookAuthorizationServer) ListMembers(w http.ResponseWriter, r *http.Request) {
	s.onOrg(w, r, func(ctx context.Context, org string) (any, error) {
		return s.Provider.ListMembers(ctx, org, ListMembersOptions{ListOptions: GetListOptions(r)})
	})
}

func (s *WebhookAuthorizationServer) GetMember(w http.ResponseWriter, r *http.Request) {
	s.onOrg(w, r, func(ctx context.Context, org string) (any, error) {
		member := Path(r, "member", "")
		if member == "" {
			return nil, errors.NewBadRequest("member is required")
		}
		return s.Provider.GetMember(ctx, org, member)
	})
}

func (s *WebhookAuthorizationServer) AddMember(w http.ResponseWriter, r *http.Request) {
	s.onOrg(w, r, func(ctx context.Context, org string) (any, error) {
		member := &Member{}
		if err := Body(r, member); err != nil {
			return nil, err
		}
		return nil, s.Provider.AddMember(ctx, org, member)
	})
}

func (s *WebhookAuthorizationServer) UpdateMember(w http.ResponseWriter, r *http.Request) {
	s.onOrg(w, r, func(ctx context.Context, org string) (any, error) {
		member := &Member{}
		if err := Body(r, member); err != nil {
			return nil, err
		}
		return nil, s.Provider.UpdateMember(ctx, org, member)
	})
}

func (s *WebhookAuthorizationServer) DeleteMember(w http.ResponseWriter, r *http.Request) {
	s.onOrg(w, r, func(ctx context.Context, org string) (any, error) {
		member := Path(r, "member", "")
		if member == "" {
			return nil, errors.NewBadRequest("member is required")
		}
		return nil, s.Provider.DeleteMember(ctx, org, member)
	})
}

func (s *WebhookAuthorizationServer) AssignRole(w http.ResponseWriter, r *http.Request) {
	s.onOrg(w, r, func(ctx context.Context, org string) (any, error) {
		member := Path(r, "member", "")
		if member == "" {
			return nil, errors.NewBadRequest("member is required")
		}
		role := Path(r, "role", "")
		if role == "" {
			return nil, errors.NewBadRequest("role is required")
		}
		return nil, s.Provider.AssignRole(ctx, org, member, role)
	})
}

func (s *WebhookAuthorizationServer) RevokeRole(w http.ResponseWriter, r *http.Request) {
	s.onOrg(w, r, func(ctx context.Context, org string) (any, error) {
		member := Path(r, "member", "")
		if member == "" {
			return nil, errors.NewBadRequest("member is required")
		}
		role := Path(r, "role", "")
		if role == "" {
			return nil, errors.NewBadRequest("role is required")
		}
		return nil, s.Provider.RevokeRole(ctx, org, member, role)
	})
}

func (s *WebhookAuthorizationServer) onOrg(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context, org string) (any, error)) {
	On(w, r, func(ctx context.Context) (any, error) {
		org := Path(r, "organization", "")
		if org == "" {
			return nil, errors.NewBadRequest("organization is required")
		}
		return fn(ctx, org)
	})
}

func (s *WebhookAuthorizationServer) Group() Group {
	return NewGroup("").
		Route(
			POST("/authorize").
				Operation("authorize").
				To(s.Authorize),

			GET("/organizations").
				Operation("list organizations").
				To(s.ListOrganizations).
				Param(PageParams...).
				Response(Page[Organization]{}, ""),

			GET("/organizations/{organization}").
				Operation("get organization").
				To(s.GetOrganization).
				Param(PathParam("organization", "The organization ID")).
				Response(Organization{}, ""),

			POST("/organizations").
				Operation("create organization").
				Param(
					BodyParam("organization", Organization{}),
				).
				To(s.CreateOrganization),

			PUT("/organizations/{organization}").
				Operation("update organization").
				Param(
					BodyParam("organization", Organization{}),
				).
				To(s.UpdateOrganization),

			DELETE("/organizations/{organization}").
				Operation("delete organization").
				Param(PathParam("organization", "The organization ID")).
				To(s.DeleteOrganization),

			HEAD("/organizations/{organization}").
				Operation("exists organization").
				Param(PathParam("organization", "The organization ID")).
				To(s.ExistsOrganization),

			GET("/users/{user}/organizations").
				Operation("list user organizations").
				Param(PageParams...).
				To(s.UserOrganizations).
				Response(Page[Organization]{}, ""),

			GET("/organizations/{organization}/roles").
				Operation("list roles").
				Param(PageParams...).
				To(s.ListRoles).
				Response(Page[Role]{}, ""),

			POST("/organizations/{organization}/roles").
				Operation("create role").
				Param(BodyParam("role", Role{})).
				To(s.CreateRole),

			GET("/organizations/{organization}/roles/{role}").
				Operation("get role").
				To(s.GetRole).
				Response(Role{}, ""),

			DELETE("/organizations/{organization}/roles/{role}").
				Operation("delete role").
				To(s.DeleteRole),

			PUT("/organizations/{organization}/roles/{role}").
				Operation("update role").
				Param(BodyParam("role", Role{})).
				To(s.UpdateRole),

			GET("/organizations/{organization}/members").
				Operation("list members").
				Param(PageParams...).
				To(s.ListMembers).
				Response(Page[Member]{}, ""),

			GET("/organizations/{organization}/members/{member}").
				Operation("get member").
				To(s.GetMember).
				Response(Member{}, ""),

			POST("/organizations/{organization}/members").
				Operation("add member").
				Param(BodyParam("member", Member{})).
				To(s.AddMember),

			PUT("/organizations/{organization}/members/{member}").
				Operation("update member").
				Param(BodyParam("member", Member{})).
				To(s.UpdateMember),

			DELETE("/organizations/{organization}/members/{member}").
				Operation("delete member").
				To(s.DeleteMember),

			PUT("/organizations/{organization}/members/{member}/roles/{role}").
				Operation("assign role to member").
				To(s.AssignRole),

			DELETE("/organizations/{organization}/members/{member}/roles/{role}").
				Operation("revoke role from member").
				To(s.RevokeRole),
		)
}
