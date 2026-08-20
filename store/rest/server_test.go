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
	getOptions        store.GetOptions
	listOptions       store.ListOptions
	patchBatchOptions store.PatchBatchOptions
	deleteOptions     store.DeleteOptions
}

func (s *optionCaptureStore) Get(_ context.Context, id string, object store.Object, opts ...store.GetOption) error {
	s.getOptions = store.ApplyGetOptions(opts)
	object.SetID(id)
	return nil
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
	options := store.ApplyListOptions(opts)
	s.listOptions = options
	store.SetContinuationListMetadata(list, "next-token", options.Limit)
	return nil
}

func (s *optionCaptureStore) PatchBatch(_ context.Context, _ store.ObjectList, _ store.PatchBatch, opts ...store.PatchBatchOption) error {
	s.patchBatchOptions = store.ApplyPatchBatchOptions(opts)
	return nil
}

func (s *optionCaptureStore) Delete(_ context.Context, _ store.Object, opts ...store.DeleteOption) error {
	s.deleteOptions = store.ApplyDeleteOptions(opts)
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
	labelRequirement := store.RequirementEqual("environment", "production")
	fieldRequirement := store.RequirementEqual("enabled", "true")

	if err := remote.List(context.Background(), list,
		store.WithPage(3, 10),
		store.WithContinuation("current-token", 2),
		store.WithLabelRequirements(labelRequirement),
		store.WithFieldRequirements(fieldRequirement),
		store.WithFields("id", "name"),
		store.WithResourceVersion(7),
		store.WithSubScopes(),
	); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got := underlying.listOptions.Continue; got != "current-token" {
		t.Fatalf("ListOptions.Continue = %q, want %q", got, "current-token")
	}
	if got := underlying.listOptions.Limit; got != 2 {
		t.Fatalf("ListOptions.Limit = %d, want 2", got)
	}
	if underlying.listOptions.Page != 3 || underlying.listOptions.Size != 10 {
		t.Fatalf("ListOptions page values = %#v", underlying.listOptions)
	}
	if !reflect.DeepEqual(underlying.listOptions.Fields, []string{"id", "name"}) {
		t.Fatalf("ListOptions.Fields = %#v", underlying.listOptions.Fields)
	}
	if underlying.listOptions.ResourceVersion == nil || *underlying.listOptions.ResourceVersion != 7 {
		t.Fatalf("ListOptions.ResourceVersion = %#v", underlying.listOptions.ResourceVersion)
	}
	if !underlying.listOptions.IncludeSubScopes {
		t.Fatal("ListOptions.IncludeSubScopes = false, want true")
	}
	if !reflect.DeepEqual(underlying.listOptions.LabelRequirements, store.Requirements{labelRequirement}) {
		t.Fatalf("ListOptions.LabelRequirements = %#v", underlying.listOptions.LabelRequirements)
	}
	if !reflect.DeepEqual(underlying.listOptions.FieldRequirements, store.Requirements{fieldRequirement}) {
		t.Fatalf("ListOptions.FieldRequirements = %#v", underlying.listOptions.FieldRequirements)
	}
	if got := list.Continue; got != "next-token" {
		t.Fatalf("List.Continue = %q, want %q", got, "next-token")
	}
	if list.Limit != 2 || list.Total != nil || list.Page != 0 || list.Size != 0 {
		t.Fatalf("List pagination metadata = %#v", list)
	}
}

func TestRemoteStoreListPreservesPage(t *testing.T) {
	underlying := &optionCaptureStore{}
	remote := newCaptureRemoteStore(t, underlying)
	list := &store.List[store.Unstructured]{Resource: "widgets"}

	if err := remote.List(context.Background(), list, store.WithPage(-2, 2)); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if underlying.listOptions.Page != -2 || underlying.listOptions.Size != 2 {
		t.Fatalf("ListOptions pagination = %#v", underlying.listOptions)
	}
}

func TestRemoteStoreGetPassesProtocolOptions(t *testing.T) {
	underlying := &optionCaptureStore{}
	remote := newCaptureRemoteStore(t, underlying)
	object := &store.Unstructured{}
	object.SetResource("widgets")

	if err := remote.Get(context.Background(), "widget-1", object,
		store.WithResourceVersion(7),
		store.WithFields("id", "name"),
	); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if underlying.getOptions.ResourceVersion == nil || *underlying.getOptions.ResourceVersion != 7 {
		t.Fatalf("GetOptions.ResourceVersion = %#v", underlying.getOptions.ResourceVersion)
	}
	if !reflect.DeepEqual(underlying.getOptions.Fields, []string{"id", "name"}) {
		t.Fatalf("GetOptions.Fields = %#v", underlying.getOptions.Fields)
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
		store.WithLabelRequirements(labelRequirement),
		store.WithFieldRequirements(fieldRequirement),
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
		store.WithLabelRequirements(labelRequirement),
		store.WithFieldRequirements(fieldRequirement),
		store.WithUID("uid-1"),
		store.WithResourceVersion(7),
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
