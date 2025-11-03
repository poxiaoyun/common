package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"xiaoshiai.cn/common/errors"
)

type Decision string

const (
	DecisionNoOpinion Decision = "NoOpinion"
	DecisionDeny      Decision = "Deny"
	DecisionAllow     Decision = "Allow"
)

var DecisionDenyStatusNotFoundMessage = "not found"

type RequestAuthorizer interface {
	AuthorizeRequest(r *http.Request) (Decision, string, error)
}

type Authorizer interface {
	Authorize(ctx context.Context, user UserInfo, a Attributes) (authorized Decision, reason string, err error)
}

func WithAuthorizationContext(ctx context.Context, decision Decision) context.Context {
	return SetContextValue(ctx, "decision", decision)
}

func AuthorizationContextFromContext(ctx context.Context) Decision {
	return GetContextValue[Decision](ctx, "decision")
}

func NewAuthorizationFilter(authorizer Authorizer) Filter {
	fn := func(r *http.Request) (Decision, string, error) {
		attributes := AttributesFromContext(r.Context())
		if attributes == nil {
			return DecisionDeny, "no attributes", nil
		}
		span := trace.SpanFromContext(r.Context())
		span.SetAttributes(
			attribute.String("authorization.action", attributes.Action),
			attribute.StringSlice("authorization.resources", func() []string {
				resources := make([]string, 0, len(attributes.Resources))
				for _, resource := range attributes.Resources {
					resources = append(resources, resource.Resource+":"+resource.Name)
				}
				return resources
			}()),
		)
		user := AuthenticateFromContext(r.Context()).User
		decision, message, err := authorizer.Authorize(r.Context(), user, *attributes)
		if err != nil {
			return DecisionDeny, message, errors.NewForbidden("", "", err)
		}
		if message == "" {
			act, res := ShowMessage(attributes)
			message = fmt.Sprintf("access denied for %s %s", act, res)
		}
		return decision, message, nil
	}
	return NewRequestAuthorizationFilter(RequestAuthorizerFunc(fn))
}

func ShowMessage(a *Attributes) (string, string) {
	res := []string{}
	for _, resource := range a.Resources {
		if resource.Resource != "" {
			res = append(res, resource.Resource)
		}
		if resource.Name != "" {
			res = append(res, resource.Name)
		}
	}
	action := a.Action
	return action, strings.Join(res, ":")
}

func NewRequestAuthorizationFilter(on RequestAuthorizer) Filter {
	return FilterFunc(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		// already authorized by previous filter
		decision := AuthorizationContextFromContext(r.Context())
		if decision == DecisionAllow {
			next.ServeHTTP(w, r)
			return
		}
		if decision == DecisionDeny {
			Forbidden(w, "access denied")
			return
		}
		decision, reason, err := on.AuthorizeRequest(r)
		if err != nil {
			// allow custom code
			Error(w, err)
			return
		}
		if decision == DecisionAllow {
			// allow next filter to skip authorization
			r = r.WithContext(WithAuthorizationContext(r.Context(), decision))
			next.ServeHTTP(w, r)
			return
		}
		if decision == DecisionDeny {
			if reason == DecisionDenyStatusNotFoundMessage {
				NotFound(w, reason)
			} else {
				if reason == "" {
					reason = "access denied"
				}
				Forbidden(w, reason)
			}
			return
		}
		// DecisionNoOpinion
		Forbidden(w, "access denied")
	})
}
