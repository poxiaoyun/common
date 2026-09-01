package api

import (
	"context"
	stderrors "errors"
	"fmt"
	"net/http"
	"strings"

	"xiaoshiai.cn/common/authz"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/meta"
)

const (
	// SystemAdminGroup identifies principals with system-wide administrative authority.
	SystemAdminGroup = "system:admin"
	// SystemBannedGroup identifies principals denied system-wide access.
	SystemBannedGroup = "system:banned"
)

// RequestAuthorizer decides whether an HTTP request may proceed.
type RequestAuthorizer interface {
	// AuthorizeRequest returns an authorization decision and an optional
	// public reason. NoOpinion means this Authorizer does not handle the
	// request. A returned [errors.Status] intentionally defines the public error
	// response; any other error is diagnostic and receives the default Forbidden
	// response.
	AuthorizeRequest(r *http.Request) (authz.EvaluationResult, error)
}

// AuthorizationFilter authorizes an authenticated principal against request
// attributes.
type AuthorizationFilter struct {
	// Authorizer evaluates the authenticated principal and request attributes.
	Authorizer authz.Authorizer
}

// RequestAuthorizationFilter authorizes an HTTP request directly.
type RequestAuthorizationFilter struct {
	// Authorizer evaluates the HTTP request.
	Authorizer RequestAuthorizer
}

var (
	_ Filter            = (*AuthorizationFilter)(nil)
	_ RequestAuthorizer = (*AuthorizationFilter)(nil)
	_ Filter            = (*RequestAuthorizationFilter)(nil)
)

// WithAuthorizationDecision records a decision for downstream authorization
// filters on the same request.
func WithAuthorizationDecision(ctx context.Context, decision authz.Decision) context.Context {
	return SetContextValue(ctx, "decision", decision)
}

// AuthorizationDecisionFromContext returns the decision recorded for the
// request. Its zero value means no previous filter made a decision.
func AuthorizationDecisionFromContext(ctx context.Context) authz.Decision {
	return GetContextValue[authz.Decision](ctx, "decision")
}

// NewAuthorizationFilter authorizes the authenticated principal against the
// Attributes installed on the request. Allow proceeds, while Deny, NoOpinion,
// and errors stop the request.
func NewAuthorizationFilter(authorizer authz.Authorizer) *AuthorizationFilter {
	return &AuthorizationFilter{Authorizer: authorizer}
}

// Process implements Filter.
func (filter *AuthorizationFilter) Process(w http.ResponseWriter, r *http.Request, next http.Handler) {
	attributes := AttributesFromContext(r.Context())
	if attributes == nil {
		Forbidden(w, "no attributes")
		return
	}

	processRequestAuthorization(w, r, next, filter)
}

// AuthorizeRequest adapts the configured domain Authorizer to RequestAuthorizer.
func (filter *AuthorizationFilter) AuthorizeRequest(r *http.Request) (authz.EvaluationResult, error) {
	attributes := AttributesFromContext(r.Context())
	authentication := AuthenticationFromContext(r.Context())
	result, err := filter.Authorizer.Authorize(r.Context(), authentication, authorizationOperation(*attributes))
	if err != nil {
		result.Decision = authz.DecisionDeny
		return result, err
	}
	if result.Decision == authz.DecisionDeny && result.Reason == "" {
		result.Reason = ForbiddenMessage(r.Context(), authentication, attributes)
	}
	return result, nil
}

// ForbiddenMessage returns the default authorization-denial message. It uses
// Path when Resources is empty and otherwise renders a colon-delimited target.
// ctx is reserved for request-scoped localization.
func ForbiddenMessage(ctx context.Context, authentication Authentication, a *Attributes) string {
	res := []string{}
	if len(a.Resources) == 0 {
		return fmt.Sprintf(
			"subject %q cannot %s path %q",
			meta.Or(authentication.Name, authentication.Email, authentication.ID),
			a.Action,
			a.Path,
		)
	}
	for _, resource := range a.Resources {
		if resource.Resource != "" {
			res = append(res, resource.Resource)
		}
		if resource.Name != "" {
			res = append(res, resource.Name)
		}
	}
	return fmt.Sprintf(
		"subject %q cannot %s resource %q",
		meta.Or(authentication.Name, authentication.Email, authentication.ID),
		a.Action,
		strings.Join(res, ":"),
	)
}

// NewRequestAuthorizationFilter applies request authorization. A previous
// Allow decision skips evaluation, a previous Deny remains denied, and a new
// NoOpinion decision is denied by default.
func NewRequestAuthorizationFilter(authorizer RequestAuthorizer) *RequestAuthorizationFilter {
	return &RequestAuthorizationFilter{Authorizer: authorizer}
}

// Process implements Filter.
func (filter *RequestAuthorizationFilter) Process(w http.ResponseWriter, r *http.Request, next http.Handler) {
	processRequestAuthorization(w, r, next, filter.Authorizer)
}

func processRequestAuthorization(w http.ResponseWriter, r *http.Request, next http.Handler, authorizer RequestAuthorizer) {
	// When multiple request-authorization filters wrap the same handler, an
	// earlier filter records its final Allow before invoking next. Reuse that
	// decision so downstream filters do not authorize the same request again.
	decision := AuthorizationDecisionFromContext(r.Context())
	if decision == authz.DecisionAllow {
		next.ServeHTTP(w, r)
		return
	}
	if decision == authz.DecisionDeny {
		Forbidden(w, "access denied")
		return
	}
	result, err := authorizer.AuthorizeRequest(r)
	if err != nil {
		// Preserve an intentional response status or authentication challenge
		// carried by the authorization error. Untyped diagnostic errors are logged
		// and receive the default Forbidden response.
		var status *errors.Status
		if stderrors.As(err, &status) {
			// A Status is an intentional public response from the authorizer.
			Error(w, err)
		} else {
			// Keep diagnostic details in logs and return the default response.
			log.FromContext(r.Context()).Error(err, "authorization failed")
			Forbidden(w, "access denied")
		}
		return
	}
	if result.Decision == authz.DecisionAllow {
		// Record the final Allow for downstream request-authorization filters.
		r = r.WithContext(WithAuthorizationDecision(r.Context(), result.Decision))
		next.ServeHTTP(w, r)
		return
	}
	if result.Decision == authz.DecisionDeny {
		if result.Reason == "" {
			result.Reason = "access denied"
		}
		Forbidden(w, result.Reason)
		return
	}
	// NoOpinion and unknown decisions fail closed.
	Forbidden(w, "access denied")
}
