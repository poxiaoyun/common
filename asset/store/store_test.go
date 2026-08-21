package store_test

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"

	"xiaoshiai.cn/common/asset"
	assetstore "xiaoshiai.cn/common/asset/store"
	"xiaoshiai.cn/common/meta"
	"xiaoshiai.cn/common/store"
	storeinmemory "xiaoshiai.cn/common/store/inmemory"
)

type listOptionsStore struct {
	store.Store
	options *store.ListOptions
}

func (s *listOptionsStore) List(ctx context.Context, list store.ObjectList, options ...store.ListOption) error {
	*s.options = store.ApplyListOptions(options)
	return s.Store.List(ctx, list, options...)
}

func (s *listOptionsStore) Scope(scopes ...store.Scope) store.Store {
	return &listOptionsStore{
		Store:   s.Store.Scope(scopes...),
		options: s.options,
	}
}

func TestAddToSchemaRegistersAssetsWithStableScopes(t *testing.T) {
	resource, err := store.GetResource(&assetstore.Asset{})
	if err != nil {
		t.Fatal(err)
	}
	if resource != "assets" {
		t.Fatalf("resource = %q, want assets", resource)
	}
	schema := store.NewSchema()
	if err := assetstore.AddToSchema(schema); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(schema.Resources(), []string{"assets"}) {
		t.Fatalf("resources = %#v", schema.Resources())
	}
	definition, err := schema.Resource("assets")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(definition.ScopeKeys, []string{"kind", "owner"}) {
		t.Fatalf("scope keys = %#v", definition.ScopeKeys)
	}
}

func TestServicePersistsContentAndIsolatesTargets(t *testing.T) {
	schema := store.NewSchema()
	if err := assetstore.AddToSchema(schema); err != nil {
		t.Fatal(err)
	}
	storage, err := storeinmemory.New(schema)
	if err != nil {
		t.Fatal(err)
	}
	service := assetstore.New(storage, assetstore.Options{})
	firstTarget := asset.Target{Kind: "application", Name: "cloud:first"}
	secondTarget := asset.Target{Kind: "application", Name: "cloud:second"}
	for _, target := range []asset.Target{firstTarget, secondTarget} {
		if _, err := service.Put(
			t.Context(),
			target,
			asset.Blob{Content: strings.NewReader(target.Name), ContentType: "text/plain"},
			asset.PutOptions{Name: "icon", Metadata: map[string]string{"owner": target.Name}},
		); err != nil {
			t.Fatal(err)
		}
	}

	page, err := service.List(t.Context(), firstTarget, meta.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Target != firstTarget || page.Items[0].Metadata["owner"] != firstTarget.Name {
		t.Fatalf("List(first target) = %#v", page)
	}
	resolved, err := service.Resolve(t.Context(), firstTarget, "icon", asset.ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(resolved.Content)
	if err != nil {
		t.Fatal(err)
	}
	resolved.Content.Close()
	if string(data) != firstTarget.Name {
		t.Fatalf("Resolve() = %q", data)
	}

	if err := service.DeleteAll(t.Context(), firstTarget); err != nil {
		t.Fatal(err)
	}
	page, err = service.List(t.Context(), secondTarget, meta.ListOptions{})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("List(second target) = %#v, %v", page, err)
	}
}

func TestServiceListDelegatesSortingAndPaginationToStore(t *testing.T) {
	schema := store.NewSchema()
	if err := assetstore.AddToSchema(schema); err != nil {
		t.Fatal(err)
	}
	storage, err := storeinmemory.New(schema)
	if err != nil {
		t.Fatal(err)
	}
	resolved := &store.ListOptions{}
	service := assetstore.New(&listOptionsStore{Store: storage, options: resolved}, assetstore.Options{})
	target := asset.Target{Kind: "application", Name: "cloud:database"}
	for _, name := range []string{"03", "01", "02"} {
		if _, err := service.Put(
			t.Context(),
			target,
			asset.Blob{Content: strings.NewReader(name), ContentType: "text/plain"},
			asset.PutOptions{Name: name},
		); err != nil {
			t.Fatal(err)
		}
	}

	page, err := service.List(t.Context(), target, meta.ListOptions{
		Page:   1,
		Size:   2,
		Search: "0",
		Sort:   "name-",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Page != 1 || resolved.Size != 2 || resolved.Search != "0" || resolved.Sort != "id-" {
		t.Fatalf("Store ListOptions = %#v", resolved)
	}
	if page.Total == nil || *page.Total != 3 || page.Page != 1 || page.Size != 2 || len(page.Items) != 2 {
		t.Fatalf("List() metadata = %#v", page)
	}
	if got := []string{page.Items[0].Name, page.Items[1].Name}; !slices.Equal(got, []string{"03", "02"}) {
		t.Fatalf("List() names = %#v, want [03 02]", got)
	}
}
