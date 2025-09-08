package api

import (
	"context"
	"net/http"

	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/store"
)

type ResponseStatusOnly int32

var (
	Empty     = ResponseStatusOnly(http.StatusOK)
	Accepted  = ResponseStatusOnly(http.StatusAccepted)
	Created   = ResponseStatusOnly(http.StatusCreated)
	NoContent = ResponseStatusOnly(http.StatusNoContent)
)

func On(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context) (any, error)) {
	ctx := r.Context()
	obj, err := fn(ctx)
	if err != nil {
		Error(w, err)
		log.FromContext(ctx).Error(err, "response")
		return
	}
	switch val := obj.(type) {
	case nil:
		// no action
	case *errors.Status:
		Raw(w, int(val.Code), val)
	case ResponseStatusOnly:
		w.WriteHeader(int(val))
	default:
		Success(w, obj)
	}
}

func OnCurrentUser(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context, user string) (any, error)) {
	On(w, r, func(ctx context.Context) (any, error) {
		userinfo := AuthenticateFromContext(ctx)
		if userinfo.User.Name == "" {
			return nil, errors.NewBadRequest("require login")
		}
		return fn(ctx, userinfo.User.Name)
	})
}

type ScopeVar struct {
	Resource    string
	PathVarName string
}

func OnScope(w http.ResponseWriter, r *http.Request, scopeVars []ScopeVar, fn func(ctx context.Context, scopes []store.Scope) (any, error)) {
	On(w, r, func(ctx context.Context) (any, error) {
		scopes := make([]store.Scope, 0, len(scopeVars))
		for _, val := range scopeVars {
			value := Path(r, val.PathVarName, "")
			if value == "" {
				return nil, errors.NewBadRequest(val.PathVarName + " is required")
			}
			scopes = append(scopes, store.Scope{Resource: val.Resource, Name: value})
		}
		return fn(ctx, scopes)
	})
}
