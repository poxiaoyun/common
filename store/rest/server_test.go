package rest

import (
	"context"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"xiaoshiai.cn/common/rest/api"
	"xiaoshiai.cn/common/store"
)

type pingStore struct {
	store.Store
	called bool
}

func (s *pingStore) Ping(context.Context) error {
	s.called = true
	return nil
}

func TestRemoteStorePing(t *testing.T) {
	underlying := &pingStore{}
	handler := api.New().Group(NewServer(underlying).Group()).Build()
	server := httptest.NewServer(handler)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if err := NewRemoteStore(serverURL).Ping(context.Background()); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
	if !underlying.called {
		t.Fatal("Ping() was not delegated to the server store")
	}
}

func Test_decodePath(t *testing.T) {
	tests := []struct {
		rpath string
		want  store.ResourcedObjectReference
	}{
		{
			rpath: "/scope1/name/scope2/name/resource/name",
			want: store.ResourcedObjectReference{
				ID:       "name",
				Resource: "resource",
				Scopes: []store.Scope{
					{Resource: "scope1", Name: "name"},
					{Resource: "scope2", Name: "name"},
				},
			},
		},
		{
			rpath: "/scope1/name/scope2/name/resource/",
			want: store.ResourcedObjectReference{
				Resource: "resource",
				Scopes: []store.Scope{
					{Resource: "scope1", Name: "name"},
					{Resource: "scope2", Name: "name"},
				},
			},
		},
		{
			rpath: "/scope1/name/scope2/name/resource",
			want: store.ResourcedObjectReference{
				Resource: "resource",
				Scopes: []store.Scope{
					{Resource: "scope1", Name: "name"},
					{Resource: "scope2", Name: "name"},
				},
			},
		},
		{
			rpath: "/resource",
			want: store.ResourcedObjectReference{
				Resource: "resource",
				Scopes:   []store.Scope{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.rpath, func(t *testing.T) {
			if got := decodePath(tt.rpath); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("decodePath() = %v, want %v", got, tt.want)
			}
		})
	}
}
