package inmemory_test

import (
	"io"
	"strings"
	"testing"
	"time"

	"xiaoshiai.cn/common/asset"
	assetinmemory "xiaoshiai.cn/common/asset/inmemory"
	commonerrors "xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/meta"
)

func TestServiceLifecycle(t *testing.T) {
	service := assetinmemory.New(assetinmemory.Options{})
	target := asset.Target{Kind: "application", Name: "cloud:database"}
	initialMetadata := map[string]string{"theme": "light"}

	first, err := service.Put(
		t.Context(),
		target,
		asset.Blob{
			Content:       strings.NewReader("first"),
			ContentType:   "text/plain",
			ContentLength: 5,
			FileName:      "first.txt",
			ModTime:       time.Date(2026, 8, 21, 1, 2, 3, 0, time.UTC),
		},
		asset.PutOptions{Name: "icon", Metadata: initialMetadata},
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.Target != target || first.Name != "icon" || first.Version != 1 || first.Metadata["theme"] != "light" {
		t.Fatalf("first Put() = %#v", first)
	}
	initialMetadata["theme"] = "changed input"
	first.Metadata["theme"] = "changed result"
	loaded, err := service.Get(t.Context(), target, "icon")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Metadata["theme"] != "light" {
		t.Fatalf("Get() metadata = %#v", loaded.Metadata)
	}

	second, err := service.Put(t.Context(), target, asset.Blob{
		Content:     strings.NewReader("second"),
		ContentType: "text/plain",
	}, asset.PutOptions{Name: "icon"})
	if err != nil {
		t.Fatal(err)
	}
	if second.Version != 2 || second.Digest == first.Digest || second.Metadata["theme"] != "light" {
		t.Fatalf("second Put() = %#v, first = %#v", second, first)
	}

	third, err := service.Put(t.Context(), target, asset.Blob{
		Content:     strings.NewReader("third"),
		ContentType: "text/plain",
	}, asset.PutOptions{Name: "icon", Metadata: map[string]string{"theme": "blue"}})
	if err != nil {
		t.Fatal(err)
	}
	if third.Version != 3 || third.Metadata["theme"] != "blue" {
		t.Fatalf("third Put() = %#v", third)
	}

	updated, err := service.ReplaceMetadata(t.Context(), target, "icon", map[string]string{"theme": "dark"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != third.Version || updated.Digest != third.Digest || updated.ETag != third.ETag || updated.Metadata["theme"] != "dark" {
		t.Fatalf("ReplaceMetadata() = %#v", updated)
	}

	page, err := service.List(t.Context(), target, meta.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Name != "icon" {
		t.Fatalf("List() = %#v", page)
	}

	resolved, err := service.Resolve(t.Context(), target, "icon", asset.ResolveOptions{Version: 3})
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(resolved.Content)
	if err != nil {
		t.Fatal(err)
	}
	resolved.Content.Close()
	if string(data) != "third" || resolved.Link != nil {
		t.Fatalf("Resolve() content = %q, link = %#v", data, resolved.Link)
	}
	ranged, err := service.Resolve(t.Context(), target, "icon", asset.ResolveOptions{Range: "bytes=1-3"})
	if err != nil {
		t.Fatal(err)
	}
	rangedData, err := io.ReadAll(ranged.Content)
	if err != nil {
		t.Fatal(err)
	}
	ranged.Content.Close()
	if string(rangedData) != "hir" || ranged.ContentLength != 3 || ranged.ContentRange != "bytes 1-3/5" {
		t.Fatalf("Resolve(range) = content %q, length %d, range %q", rangedData, ranged.ContentLength, ranged.ContentRange)
	}
	if _, err := service.Resolve(t.Context(), target, "icon", asset.ResolveOptions{Version: 1}); !commonerrors.IsNotFound(err) {
		t.Fatalf("Resolve(stale) error = %v, want NotFound", err)
	}

	if err := service.DeleteAll(t.Context(), target); err != nil {
		t.Fatal(err)
	}
	page, err = service.List(t.Context(), target, meta.ListOptions{})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("List(after delete) = %#v, %v", page, err)
	}
}

func TestServiceAppliesNamedPolicy(t *testing.T) {
	service := assetinmemory.New(assetinmemory.Options{Policies: assetinmemory.Policies{
		{}:                             {MaxBytes: assetinmemory.DefaultMaxBytes},
		{Kind: "user", Name: "avatar"}: {MaxBytes: 3},
	}})
	_, err := service.Put(
		t.Context(),
		asset.Target{Kind: "user", Name: "alice"},
		asset.Blob{Content: strings.NewReader("large"), ContentType: "text/plain"},
		asset.PutOptions{Name: "avatar"},
	)
	if !commonerrors.IsCode(err, 413) {
		t.Fatalf("Put() error = %v, want 413", err)
	}
}

func TestServiceAppliesWildcardMediaTypePolicy(t *testing.T) {
	service := assetinmemory.New(assetinmemory.Options{Policies: assetinmemory.Policies{
		{Kind: "user", Name: "avatar"}: {
			MaxBytes:          assetinmemory.DefaultMaxBytes,
			AllowedMediaTypes: []string{"image/*"},
		},
	}})
	target := asset.Target{Kind: "user", Name: "alice"}
	if _, err := service.Put(
		t.Context(),
		target,
		asset.Blob{Content: strings.NewReader("<svg></svg>"), ContentType: "image/svg+xml"},
		asset.PutOptions{Name: "avatar"},
	); err != nil {
		t.Fatalf("Put(SVG) error = %v", err)
	}
	_, err := service.Put(
		t.Context(),
		target,
		asset.Blob{Content: strings.NewReader("text"), ContentType: "text/plain"},
		asset.PutOptions{Name: "avatar"},
	)
	if !commonerrors.IsCode(err, 400) {
		t.Fatalf("Put(text) error = %v, want 400", err)
	}
}

func TestServiceStoresDirectLink(t *testing.T) {
	service := assetinmemory.New(assetinmemory.Options{})
	target := asset.Target{Kind: "application", Name: "cloud:database"}
	created, err := service.Put(t.Context(), target, asset.Blob{
		Link:          &asset.Link{URL: "https://objects.example/icon"},
		ContentType:   "image/png",
		ContentLength: 123,
		FileName:      "icon.png",
	}, asset.PutOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if created.Name == "" || created.FileName != "icon.png" || created.Size != 123 {
		t.Fatalf("Put() = %#v", created)
	}
	resolved, err := service.Resolve(t.Context(), target, created.Name, asset.ResolveOptions{Range: "bytes=0-9"})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Content != nil || resolved.Link == nil || resolved.Link.URL != "https://objects.example/icon" {
		t.Fatalf("Resolve() = %#v", resolved)
	}
}

func TestServiceRejectsIncorrectContentLength(t *testing.T) {
	service := assetinmemory.New(assetinmemory.Options{})
	_, err := service.Put(t.Context(), asset.Target{Kind: "user", Name: "alice"}, asset.Blob{
		Content:       strings.NewReader("content"),
		ContentType:   "text/plain",
		ContentLength: 8,
	}, asset.PutOptions{Name: "avatar"})
	if !commonerrors.IsCode(err, 400) {
		t.Fatalf("Put() error = %v, want 400", err)
	}
}
