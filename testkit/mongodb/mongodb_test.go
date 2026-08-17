package mongodb_test

import (
	"testing"

	"xiaoshiai.cn/common/testkit/mongodb"
)

func TestRequireURIUsesConfiguredURI(t *testing.T) {
	want := "mongodb://example.test:27017/?replicaSet=cluster"
	t.Setenv("MONGODB_URI", want)

	if uri := mongodb.RequireURI(t); uri != want {
		t.Fatalf("RequireURI() = %q, want %q", uri, want)
	}
}
