package postgresql_test

import (
	"testing"

	testpostgresql "xiaoshiai.cn/common/testkit/postgresql"
)

func TestRequireURIUsesConfiguredURI(t *testing.T) {
	want := "postgres://postgres:secret@example.test:5432/common"
	t.Setenv("POSTGRESQL_URI", want)
	if got := testpostgresql.RequireURI(t); got != want {
		t.Fatalf("RequireURI() = %q, want %q", got, want)
	}
}
