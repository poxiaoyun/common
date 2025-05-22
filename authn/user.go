package authn

import (
	"context"
	"net/http"

	"k8s.io/apimachinery/pkg/api/errors"
	"xiaoshiai.cn/common/rand"
	"xiaoshiai.cn/common/rest/api"
)

func (a *API) ListUsers(w http.ResponseWriter, r *http.Request) {
	api.On(w, r, func(ctx context.Context) (any, error) {
		return a.Provider.ListUsers(ctx, ListUserOptions{ListOptions: api.GetListOptions(r)})
	})
}

func (a *API) GetUser(w http.ResponseWriter, r *http.Request) {
	a.OnUser(w, r, func(ctx context.Context, user string) (any, error) {
		return a.Provider.GetUser(ctx, user)
	})
}

type UserProfileWithPassword struct {
	UserProfile `json:",inline"`
	Password    string `json:"password"`
}

func (a *API) CreateUser(w http.ResponseWriter, r *http.Request) {
	api.On(w, r, func(ctx context.Context) (any, error) {
		profilewithPassword := &UserProfileWithPassword{}
		if err := api.Body(r, profilewithPassword); err != nil {
			return nil, err
		}
		if profilewithPassword.Password == "" {
			profilewithPassword.Password = rand.RandomPassword(16)
		}
		if err := a.Provider.CreateUser(ctx, &profilewithPassword.UserProfile, profilewithPassword.Password); err != nil {
			return nil, err
		}
		return profilewithPassword, nil
	})
}

func (a *API) UpdateUser(w http.ResponseWriter, r *http.Request) {
	a.OnUser(w, r, func(ctx context.Context, user string) (any, error) {
		profile := &UserProfile{}
		if err := api.Body(r, profile); err != nil {
			return nil, err
		}
		profile.User.Name = user
		if err := a.Provider.UpdateUser(ctx, profile); err != nil {
			return nil, err
		}
		return profile, nil
	})
}

func (a *API) DeleteUser(w http.ResponseWriter, r *http.Request) {
	a.OnUser(w, r, func(ctx context.Context, user string) (any, error) {
		return a.Provider.DeleteUser(ctx, user)
	})
}

type ResetPasswordRequest struct {
	Passowrd string `json:"password"`
}

func (a *API) ResetUserPassword(w http.ResponseWriter, r *http.Request) {
	a.OnUser(w, r, func(ctx context.Context, username string) (any, error) {
		req := &ResetPasswordRequest{}
		if r.ContentLength > 0 {
			if err := api.Body(r, req); err != nil {
				return nil, errors.NewBadRequest(err.Error())
			}
		} else {
			req.Passowrd = rand.RandomPassword(16)
		}
		if err := a.Provider.SetUserPassword(ctx, username, req.Passowrd); err != nil {
			return nil, err
		}
		return req, nil
	})
}

func (a *API) GetUserProfile(w http.ResponseWriter, r *http.Request) {
	a.OnUser(w, r, func(ctx context.Context, user string) (any, error) {
		return a.Provider.GetUserProfile(ctx, user)
	})
}

func (a *API) UpdateUserProfile(w http.ResponseWriter, r *http.Request) {
	a.OnUser(w, r, func(ctx context.Context, username string) (any, error) {
		user := &UserProfile{}
		if err := api.Body(r, user); err != nil {
			return nil, err
		}
		user.Name = username
		if err := a.Provider.UpdateUserProfile(ctx, user); err != nil {
			return nil, err
		}
		return user, nil
	})
}

type NameOnly struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func (a *API) SearchUsers(w http.ResponseWriter, r *http.Request) {
	api.On(w, r, func(ctx context.Context) (any, error) {
		search := api.Query(r, "search", "")
		if search == "" {
			return []NameOnly{}, nil
		}
		list, err := a.Provider.ListUsers(ctx, ListUserOptions{ListOptions: api.ListOptions{
			Search: search, Size: 5,
		}})
		if err != nil {
			return nil, err
		}
		result := []NameOnly{}
		for _, user := range list.Items {
			result = append(result, NameOnly{Name: user.Name, Email: user.Email})
		}
		return result, nil
	})
}

func (a *API) DisableUser(w http.ResponseWriter, r *http.Request) {
	a.OnUser(w, r, func(ctx context.Context, user string) (any, error) {
		return nil, a.Provider.SetUserDisabled(ctx, user, true)
	})
}

func (a *API) EnableUser(w http.ResponseWriter, r *http.Request) {
	a.OnUser(w, r, func(ctx context.Context, user string) (any, error) {
		return nil, a.Provider.SetUserDisabled(ctx, user, false)
	})
}

func (a *API) OnUser(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context, user string) (any, error)) {
	api.On(w, r, func(ctx context.Context) (any, error) {
		user := api.Path(r, "user", "")
		if user == "" {
			return nil, errors.NewBadRequest("user is required")
		}
		return fn(ctx, user)
	})
}

func (a *API) UsersGroup() api.Group {
	return api.
		NewGroup("/users").
		Tag("Users").
		Route(
			api.GET("").
				Doc("List users").
				To(a.ListUsers).
				Param(api.PageParams...).Response(api.Page[User]{}),

			api.POST("").
				To(a.CreateUser).
				Doc("Create an new user").
				Param(api.BodyParam("user", UserProfileWithPassword{})).
				Response(UserProfileWithPassword{}),

			api.GET("/{user}").
				Doc("Get user").
				To(a.GetUser).Response(User{}),

			api.PUT("/{user}").
				Doc("Update user").
				To(a.UpdateUser).Param(api.BodyParam("user", User{})).Response(User{}),

			api.DELETE("/{user}").
				Doc("Delete user").
				To(a.DeleteUser),

			api.POST("/{user}/password").
				Param(
					api.BodyParam("password", ResetPasswordRequest{}),
				).
				Doc("Reset user password with a random password").
				To(a.ResetUserPassword).
				Response(ResetPasswordRequest{}),

			api.POST("/{user}:disable").
				Doc("Disable user").
				To(a.DisableUser),

			api.POST("/{user}:enable").
				Doc("Enable user").
				To(a.EnableUser),

			api.GET("/{user}/profile").
				Doc("Get user profile").
				To(a.GetUserProfile),

			api.PUT("/{user}/profile").
				Doc("Update user profile").
				Param(api.BodyParam("profile", UserProfile{})).
				To(a.UpdateUserProfile),
		)
}

func (a *API) UersSearchGroup() api.Group {
	return api.
		NewGroup("/user-search").
		Tag("Users").
		Route(
			api.GET("").
				Doc("Search users by name").
				To(a.SearchUsers).Param(api.PageParams...).Response([]NameOnly{}),
		)
}

func (a *API) UserProviderGroup() api.Group {
	return api.
		NewGroup("").
		Tag("User").
		SubGroup(
			a.UsersGroup(),
			a.UersSearchGroup(),
		)
}
