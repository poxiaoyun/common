package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/meta"
)

// Decision is an Authorizer's opinion about one request. DecisionAllow and
// DecisionDeny are final decisions; DecisionNoOpinion lets another Authorizer
// decide.
type Decision string

const (
	// DecisionNoOpinion indicates that the Authorizer does not handle the
	// request. An authorization chain may continue with its next Authorizer.
	DecisionNoOpinion Decision = "NoOpinion"
	// DecisionDeny rejects the request and stops an authorization chain.
	DecisionDeny Decision = "Deny"
	// DecisionAllow authorizes the request and stops an authorization chain.
	DecisionAllow Decision = "Allow"
)

// DecisionDenyStatusNotFoundMessage asks NewRequestAuthorizationFilter to hide
// a denied resource behind an HTTP 404 response.
var DecisionDenyStatusNotFoundMessage = "not found"

// RequestAuthorizer decides whether an HTTP request may proceed.
type RequestAuthorizer interface {
	// AuthorizeRequest returns an authorization decision and an optional
	// human-readable reason. DecisionNoOpinion means this Authorizer does not
	// handle the request. An error means evaluation failed and must not allow the
	// request.
	AuthorizeRequest(r *http.Request) (Decision, string, error)
}

// Authorizer decides whether an authenticated principal may perform an API
// operation.
type Authorizer interface {
	// Authorize returns an authorization decision and an optional human-readable
	// reason. DecisionNoOpinion means this Authorizer does not handle the
	// principal or operation. An error means evaluation failed and must not allow
	// the request.
	Authorize(ctx context.Context, user UserInfo, a Attributes) (authorized Decision, reason string, err error)
}

// WithAuthorizationContext records a decision for downstream authorization
// filters on the same request.
func WithAuthorizationContext(ctx context.Context, decision Decision) context.Context {
	return SetContextValue(ctx, "decision", decision)
}

// AuthorizationContextFromContext returns the decision recorded for the
// request. Its zero value means no previous filter made a decision.
func AuthorizationContextFromContext(ctx context.Context) Decision {
	return GetContextValue[Decision](ctx, "decision")
}

// NewAuthorizationFilter authorizes the authenticated principal against the
// Attributes installed on the request. Allow proceeds, while Deny, NoOpinion,
// and errors stop the request.
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
			return DecisionDeny, message, errors.NewForbidden(err)
		}
		if message == "" {
			act, res := forbiddenMessage(attributes)
			message = fmt.Sprintf("User %s cannot %s %s", meta.Or(user.Name, user.Email, user.ID), act, res)
		}
		if decision == DecisionDeny && IsOAuth2ClientPrincipal(user) {
			SetOAuth2InsufficientScopeChallenge(ResponseHeaderFromContext(r.Context()))
		}
		return decision, message, nil
	}
	return NewRequestAuthorizationFilter(RequestAuthorizerFunc(fn))
}

func forbiddenMessage(a *Attributes) (string, string) {
	res := []string{}
	if len(a.Resources) == 0 {
		return a.Action, a.Path
	}
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

// NewRequestAuthorizationFilter applies request authorization. A previous
// Allow decision skips evaluation, a previous Deny remains denied, and a new
// NoOpinion decision is denied by default.
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
