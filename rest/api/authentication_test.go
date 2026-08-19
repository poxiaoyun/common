package api_test

import (
	"context"
	"reflect"
	"testing"

	"xiaoshiai.cn/common/rest/api"
)

func TestAuthenticationAudienceContext(t *testing.T) {
	want := []string{"urn:apps:api", "urn:cloud:api"}
	ctx := api.WithAuthenticationAudiences(context.Background(), want)
	if got := api.AuthenticationAudiencesFromContext(ctx); !reflect.DeepEqual(got, want) {
		t.Fatalf("AuthenticationAudiencesFromContext() = %#v, want %#v", got, want)
	}
}
