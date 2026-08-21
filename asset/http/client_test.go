package http_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"xiaoshiai.cn/common/asset"
	assethttp "xiaoshiai.cn/common/asset/http"
	assetinmemory "xiaoshiai.cn/common/asset/inmemory"
	"xiaoshiai.cn/common/meta"
	"xiaoshiai.cn/common/rest/api"
)

func TestClientImplementsServiceProtocol(t *testing.T) {
	local := assetinmemory.New(assetinmemory.Options{})
	server := assethttp.NewServer(local)
	handler := api.New().Group(server.Group(), server.PublicGroup()).Build()
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer httpServer.Close()

	client, err := assethttp.New(t.Context(), assethttp.Options{
		Address: httpServer.URL,
		Token:   "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	target := asset.Target{Kind: "application", Name: "cloud:database"}
	created, err := client.Put(
		t.Context(),
		target,
		asset.Blob{
			Content:       strings.NewReader("content"),
			ContentType:   "text/plain",
			ContentLength: 7,
			FileName:      "icon.txt",
			ModTime:       time.Date(2026, 8, 21, 1, 2, 3, 0, time.UTC),
		},
		asset.PutOptions{Name: "icon", Metadata: map[string]string{"theme": "light"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if created.Target != target || created.Name != "icon" || created.Version != 1 || created.Metadata["theme"] != "light" {
		t.Fatalf("Put() = %#v", created)
	}

	loaded, err := client.Get(t.Context(), target, "icon")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Target != target || loaded.Digest != created.Digest || loaded.Metadata["theme"] != "light" || loaded.FileName != "icon.txt" || !loaded.ModTime.Equal(&created.ModTime) {
		t.Fatalf("Get() = %#v, want digest %q", loaded, created.Digest)
	}

	updated, err := client.ReplaceMetadata(
		t.Context(),
		target,
		"icon",
		map[string]string{"theme": "dark"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != created.Version || updated.Digest != created.Digest || updated.ETag != created.ETag || updated.Metadata["theme"] != "dark" {
		t.Fatalf("ReplaceMetadata() = %#v", updated)
	}

	page, err := client.List(t.Context(), target, meta.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Target != target {
		t.Fatalf("List() = %#v", page)
	}

	resolved, err := client.Resolve(
		t.Context(),
		target,
		"icon",
		asset.ResolveOptions{Version: created.Version},
	)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(resolved.Content)
	if err != nil {
		t.Fatal(err)
	}
	resolved.Content.Close()
	if string(data) != "content" || resolved.Link != nil {
		t.Fatalf("Resolve() content = %q, link = %#v", data, resolved.Link)
	}
	if !reflect.DeepEqual(resolved.Asset, *updated) {
		t.Fatalf("Resolve() asset = %#v, want %#v", resolved.Asset, *updated)
	}
	ranged, err := client.Resolve(
		t.Context(),
		target,
		"icon",
		asset.ResolveOptions{Range: "bytes=1-3"},
	)
	if err != nil {
		t.Fatal(err)
	}
	rangedData, err := io.ReadAll(ranged.Content)
	if err != nil {
		t.Fatal(err)
	}
	ranged.Content.Close()
	if string(rangedData) != "ont" || ranged.ContentLength != 3 || ranged.ContentRange != "bytes 1-3/7" {
		t.Fatalf("Resolve(range) = content %q, length %d, range %q", rangedData, ranged.ContentLength, ranged.ContentRange)
	}

	if err := client.DeleteAll(t.Context(), target); err != nil {
		t.Fatal(err)
	}
	page, err = client.List(t.Context(), target, meta.ListOptions{})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("List(after delete) = %#v, %v", page, err)
	}
}

func TestClientPutDirectLink(t *testing.T) {
	local := assetinmemory.New(assetinmemory.Options{})
	server := assethttp.NewServer(local)
	httpServer := httptest.NewServer(api.New().Group(server.Group(), server.PublicGroup()).Build())
	defer httpServer.Close()
	client, err := assethttp.New(t.Context(), assethttp.Options{Address: httpServer.URL})
	if err != nil {
		t.Fatal(err)
	}
	target := asset.Target{Kind: "application", Name: "cloud:database"}
	created, err := client.Put(t.Context(), target, asset.Blob{
		Link:          &asset.Link{URL: "https://objects.example/icon"},
		ContentType:   "image/png",
		ContentLength: 123,
		FileName:      "icon.png",
	}, asset.PutOptions{Metadata: map[string]string{"theme": "dark"}})
	if err != nil {
		t.Fatal(err)
	}
	if created.Name == "" || created.FileName != "icon.png" || created.Size != 123 || created.Metadata["theme"] != "dark" {
		t.Fatalf("Put() = %#v", created)
	}
	resolved, err := client.Resolve(t.Context(), target, created.Name, asset.ResolveOptions{Range: "bytes=0-9"})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Content != nil || resolved.Link == nil || resolved.Link.URL != "https://objects.example/icon" || resolved.Asset.Size != 123 {
		t.Fatalf("Resolve() = %#v", resolved)
	}
}

func TestClientResolveReturnsLinkWithoutFollowingRedirect(t *testing.T) {
	local := assetinmemory.New(assetinmemory.Options{})
	server := assethttp.NewServer(linkService{
		Service:   local,
		location:  "https://objects.example/icon?signature=secret",
		expiresAt: time.Date(2026, 8, 21, 1, 2, 3, 0, time.UTC),
	})
	httpServer := httptest.NewServer(api.New().Group(server.PublicGroup()).Build())
	defer httpServer.Close()
	client, err := assethttp.New(t.Context(), assethttp.Options{Address: httpServer.URL})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := client.Resolve(
		t.Context(),
		asset.Target{Kind: "application", Name: "cloud:database"},
		"icon",
		asset.ResolveOptions{PreferLink: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Content != nil || resolved.Link == nil || resolved.Link.URL != "https://objects.example/icon?signature=secret" || !resolved.Link.ExpiresAt.Equal(time.Date(2026, 8, 21, 1, 2, 3, 0, time.UTC)) {
		t.Fatalf("Resolve() = %#v", resolved)
	}
}

type linkService struct {
	asset.Service
	location  string
	expiresAt time.Time
}

func (s linkService) Resolve(context.Context, asset.Target, string, asset.ResolveOptions) (*asset.Resolved, error) {
	return &asset.Resolved{Link: &asset.Link{URL: s.location, ExpiresAt: s.expiresAt}}, nil
}
