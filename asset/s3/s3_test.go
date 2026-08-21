package s3_test

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"xiaoshiai.cn/common/asset"
	assets3 "xiaoshiai.cn/common/asset/s3"
	commons3 "xiaoshiai.cn/common/s3"
)

func TestServiceLifecycle(t *testing.T) {
	address := os.Getenv("COMMON_TEST_S3_ADDRESS")
	bucket := os.Getenv("COMMON_TEST_S3_BUCKET")
	if address == "" || bucket == "" {
		t.Skip("set COMMON_TEST_S3_ADDRESS and COMMON_TEST_S3_BUCKET to run")
	}
	options := commons3.NewDefaultOptions()
	options.Address = address
	options.AccessKey = os.Getenv("COMMON_TEST_S3_ACCESS_KEY")
	options.SecretKey = os.Getenv("COMMON_TEST_S3_SECRET_KEY")
	if region := os.Getenv("COMMON_TEST_S3_REGION"); region != "" {
		options.Region = region
	}
	client, err := commons3.NewClient(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "common-asset-tests/" + uuid.NewString()
	service := assets3.New(client, assets3.Options{
		Bucket: bucket,
		Prefix: prefix,
	})
	target := asset.Target{Kind: "application", Name: "test:" + uuid.NewString()}
	t.Cleanup(func() {
		if err := service.DeleteAll(context.Background(), target); err != nil {
			t.Error(err)
		}
	})

	created, err := service.Put(
		t.Context(),
		target,
		asset.Blob{Content: strings.NewReader("content"), ContentType: "text/plain"},
		asset.PutOptions{Name: "icon", Metadata: map[string]string{"theme": "light"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if created.Target != target || created.Version != 1 || created.Metadata["theme"] != "light" {
		t.Fatalf("Put() = %#v", created)
	}
	updated, err := service.ReplaceMetadata(t.Context(), target, "icon", map[string]string{"theme": "dark"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != created.Version || updated.Digest != created.Digest || updated.ETag != created.ETag || updated.Metadata["theme"] != "dark" {
		t.Fatalf("ReplaceMetadata() = %#v", updated)
	}
	loaded, err := service.Get(t.Context(), target, "icon")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ETag != created.ETag || loaded.Metadata["theme"] != "dark" {
		t.Fatalf("Get(after ReplaceMetadata) = %#v", loaded)
	}
	linked, err := service.Resolve(t.Context(), target, "icon", asset.ResolveOptions{PreferLink: true})
	if err != nil {
		t.Fatal(err)
	}
	if linked.Link == nil || linked.Content != nil {
		t.Fatalf("Resolve(link) = %#v", linked)
	}
	direct, err := service.Resolve(t.Context(), target, "icon", asset.ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(direct.Content)
	if err != nil {
		t.Fatal(err)
	}
	direct.Content.Close()
	if string(data) != "content" || direct.Link != nil {
		t.Fatalf("Resolve(content) = %q, link = %#v", data, direct.Link)
	}
	proxy := assets3.New(client, assets3.Options{
		Bucket: bucket,
		Prefix: prefix,
		Proxy:  true,
	})
	proxied, err := proxy.Resolve(t.Context(), target, "icon", asset.ResolveOptions{PreferLink: true})
	if err != nil {
		t.Fatal(err)
	}
	proxiedData, err := io.ReadAll(proxied.Content)
	if err != nil {
		t.Fatal(err)
	}
	proxied.Content.Close()
	if string(proxiedData) != "content" || proxied.Link != nil {
		t.Fatalf("Resolve(proxy) = content %q, link %#v", proxiedData, proxied.Link)
	}
}
