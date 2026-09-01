package api_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"xiaoshiai.cn/common/authn"
	"xiaoshiai.cn/common/authz"
	"xiaoshiai.cn/common/rest/api"
)

func TestCheckerAuthorizerMapsOperationAndDecision(t *testing.T) {
	authentication := authn.Authentication{Subject: authn.Subject{Type: authn.SubjectTypeUser, ID: "alice"}}
	operation := authz.Operation{
		Service: "moha",
		Action:  "list",
		Resource: authz.Resource{
			Type:  "repositories",
			Scope: authz.Scope{{Type: "organizations", ID: "engineering"}},
		},
	}
	wantCheckedOperation := authz.Operation{
		Service: "moha",
		Action:  "list",
		Resource: authz.Resource{
			Type:  "moha.repository",
			ID:    "repository",
			Scope: authz.Scope{{Type: "iam.organization", ID: "engineering"}},
			Properties: authz.Properties{
				"source": "operation-gate",
			},
		},
	}
	checker := &recordingChecker{result: authz.EvaluationResult{
		Decision: authz.DecisionAllow,
		Reason:   "organization member",
	}}
	authorizer := api.NewCheckerAuthorizer(checker, func(_ context.Context, gotAuthentication api.Authentication, gotOperation authz.Operation) (authz.Operation, error) {
		if !reflect.DeepEqual(gotAuthentication, authentication) || !reflect.DeepEqual(gotOperation, operation) {
			t.Fatalf("Authorize() input = (%#v, %#v), want (%#v, %#v)", gotAuthentication, gotOperation, authentication, operation)
		}
		return wantCheckedOperation, nil
	})

	result, err := authorizer.Authorize(t.Context(), authentication, operation)
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != authz.DecisionAllow || result.Reason != "organization member" {
		t.Fatalf("Authorize() = %#v, want Allow with reason", result)
	}
	if !reflect.DeepEqual(checker.authentication, authentication) || !reflect.DeepEqual(checker.operation, wantCheckedOperation) {
		t.Fatalf("Check() input = (%#v, %#v), want (%#v, %#v)", checker.authentication, checker.operation, authentication, wantCheckedOperation)
	}
}

func TestCheckerAuthorizerFailsClosed(t *testing.T) {
	diagnostic := errors.New("policy decision point unavailable")
	tests := []struct {
		name       string
		result     authz.EvaluationResult
		checkErr   error
		wantReason string
		wantErr    error
	}{
		{
			name:       "deny",
			result:     authz.EvaluationResult{Decision: authz.DecisionDeny, Reason: "not a member"},
			wantReason: "not a member",
		},
		{
			name:   "unknown decision",
			result: authz.EvaluationResult{Decision: authz.Decision("conditional")},
		},
		{
			name:     "check error",
			checkErr: diagnostic,
			wantErr:  diagnostic,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := &recordingChecker{result: tt.result, err: tt.checkErr}
			authorizer := api.NewCheckerAuthorizer(checker, func(context.Context, api.Authentication, authz.Operation) (authz.Operation, error) {
				return authz.Operation{}, nil
			})

			result, err := authorizer.Authorize(t.Context(), api.Authentication{}, authz.Operation{})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Authorize() error = %v, want %v", err, tt.wantErr)
			}
			if result.Decision != authz.DecisionDeny || result.Reason != tt.wantReason {
				t.Fatalf("Authorize() = %#v, want Deny with reason %q", result, tt.wantReason)
			}
		})
	}
}

func TestCheckerAuthorizerReturnsMappingErrorWithoutCheck(t *testing.T) {
	diagnostic := errors.New("resource mapping failed")
	checker := &recordingChecker{}
	authorizer := api.NewCheckerAuthorizer(checker, func(context.Context, api.Authentication, authz.Operation) (authz.Operation, error) {
		return authz.Operation{}, diagnostic
	})

	result, err := authorizer.Authorize(t.Context(), api.Authentication{}, authz.Operation{})
	if !errors.Is(err, diagnostic) {
		t.Fatalf("Authorize() error = %v, want %v", err, diagnostic)
	}
	if result.Decision != authz.DecisionDeny || result.Reason != "" {
		t.Fatalf("Authorize() = %#v, want Deny with empty reason", result)
	}
	if checker.called {
		t.Fatal("Checker.Check() was called after mapping failed")
	}
}

type recordingChecker struct {
	authentication authn.Authentication
	operation      authz.Operation
	result         authz.EvaluationResult
	err            error
	called         bool
}

func (checker *recordingChecker) Check(_ context.Context, authentication authn.Authentication, operation authz.Operation, _ ...authz.CheckOption) (authz.EvaluationResult, error) {
	checker.called = true
	checker.authentication = authentication
	checker.operation = operation
	return checker.result, checker.err
}

var _ authz.Checker = (*recordingChecker)(nil)
