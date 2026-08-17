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

type optionCaptureStore struct {
	store.Store
	listOptions       store.ListOptions
	patchBatchOptions store.PatchBatchOptions
	deleteOptions     store.DeleteOptions
}

type remoteSchemaObject struct {
	store.ObjectMeta `json:",inline"`
}

func (*remoteSchemaObject) ResourceName() string { return "remote-schema-tests" }

type remoteSchemaMutationObject struct {
	store.ObjectMeta `json:",inline"`
}

func (*remoteSchemaMutationObject) ResourceName() string { return "remote-schema-mutations" }

func (s *optionCaptureStore) Scope(...store.Scope) store.Store {
	return s
}

func (s *optionCaptureStore) List(_ context.Context, list store.ObjectList, opts ...store.ListOption) error {
	options := store.ListOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	s.listOptions = options
	list.SetContinue("next-token")
	return nil
}

func (s *optionCaptureStore) PatchBatch(_ context.Context, _ store.ObjectList, _ store.PatchBatch, opts ...store.PatchBatchOption) error {
	options := store.PatchBatchOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	s.patchBatchOptions = options
	return nil
}

func (s *optionCaptureStore) Delete(_ context.Context, _ store.Object, opts ...store.DeleteOption) error {
	options := store.DeleteOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	s.deleteOptions = options
	return nil
}

func newCaptureRemoteStore(t *testing.T, underlying store.Store) *Client {
	t.Helper()
	serverAPI := NewServer(underlying)
	handler := api.New().
		Group(serverAPI.Group()).
		Build()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	return NewRemoteStore(serverURL, store.NewSchema())
}

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
	remote := newCaptureRemoteStore(t, underlying)
	if err := remote.Ping(context.Background()); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
	if !underlying.called {
		t.Fatal("Ping() was not delegated to the server store")
	}
}

func TestRemoteStoreSchemaSnapshot(t *testing.T) {
	schema := store.NewSchema()
	if err := schema.Register(&remoteSchemaObject{}, store.ResourceSchema{}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	serverURL, err := url.Parse("https://store.example")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	remote := NewRemoteStore(serverURL, schema)
	if err := schema.Register(&remoteSchemaMutationObject{}, store.ResourceSchema{}); err != nil {
		t.Fatalf("source schema Register() error = %v", err)
	}

	scoped := remote.Scope(store.Scope{Resource: "tenants", Name: "tenant-a"})
	snapshot := scoped.Schema()
	if err := snapshot.Register(&remoteSchemaMutationObject{}, store.ResourceSchema{}); err != nil {
		t.Fatalf("Schema().Register() error = %v", err)
	}
	remoteSchema := remote.Schema()
	if got := remoteSchema.Resources(); !reflect.DeepEqual(got, []string{"remote-schema-tests"}) {
		t.Fatalf("Schema().Resources() = %v, want [remote-schema-tests]", got)
	}
}

func TestRemoteStoreListPassesContinue(t *testing.T) {
	underlying := &optionCaptureStore{}
	remote := newCaptureRemoteStore(t, underlying)
	list := &store.List[store.Unstructured]{Resource: "widgets"}

	if err := remote.List(context.Background(), list,
		store.WithPageSize(0, 2),
		store.WithContinue("current-token"),
	); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got := underlying.listOptions.Continue; got != "current-token" {
		t.Fatalf("ListOptions.Continue = %q, want %q", got, "current-token")
	}
	if got := list.Continue; got != "next-token" {
		t.Fatalf("List.Continue = %q, want %q", got, "next-token")
	}
}

func TestRemoteStorePatchBatchPassesSelectors(t *testing.T) {
	underlying := &optionCaptureStore{}
	remote := newCaptureRemoteStore(t, underlying)
	list := &store.List[store.Unstructured]{Resource: "widgets"}
	labelRequirement := store.RequirementEqual("environment", "test")
	fieldRequirement := store.RequirementEqual("enabled", "true")

	err := remote.PatchBatch(context.Background(), list,
		store.MapMergePatchBacth{"enabled": false},
		store.WithPatchBatchLabelRequirements(labelRequirement),
		store.WithPatchBatchFieldRequirements(fieldRequirement),
	)
	if err != nil {
		t.Fatalf("PatchBatch() error = %v", err)
	}
	if !reflect.DeepEqual(underlying.patchBatchOptions.LabelRequirements, store.Requirements{labelRequirement}) {
		t.Fatalf("LabelRequirements = %#v, want %#v",
			underlying.patchBatchOptions.LabelRequirements, store.Requirements{labelRequirement})
	}
	if !reflect.DeepEqual(underlying.patchBatchOptions.FieldRequirements, store.Requirements{fieldRequirement}) {
		t.Fatalf("FieldRequirements = %#v, want %#v",
			underlying.patchBatchOptions.FieldRequirements, store.Requirements{fieldRequirement})
	}
}

func TestRemoteStoreDeletePassesConditions(t *testing.T) {
	underlying := &optionCaptureStore{}
	remote := newCaptureRemoteStore(t, underlying)
	object := &store.Unstructured{}
	object.SetResource("widgets")
	object.SetID("widget-1")
	labelRequirement := store.RequirementEqual("environment", "test")
	fieldRequirement := store.RequirementEqual("enabled", "true")

	if err := remote.Delete(context.Background(), object,
		store.WithDeleteLabelRequirements(labelRequirement),
		store.WithDeleteFieldRequirements(fieldRequirement),
		store.WithDeleteUID("uid-1"),
		store.WithDeleteResourceVersion(7),
	); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !reflect.DeepEqual(underlying.deleteOptions.LabelRequirements, store.Requirements{labelRequirement}) {
		t.Fatalf("LabelRequirements = %#v", underlying.deleteOptions.LabelRequirements)
	}
	if !reflect.DeepEqual(underlying.deleteOptions.FieldRequirements, store.Requirements{fieldRequirement}) {
		t.Fatalf("FieldRequirements = %#v", underlying.deleteOptions.FieldRequirements)
	}
	if underlying.deleteOptions.Preconditions == nil ||
		*underlying.deleteOptions.Preconditions.UID != "uid-1" ||
		*underlying.deleteOptions.Preconditions.ResourceVersion != 7 {
		t.Fatalf("Preconditions = %#v", underlying.deleteOptions.Preconditions)
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
