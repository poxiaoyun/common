package mysql_test

import (
	"testing"

	testmysql "xiaoshiai.cn/common/testkit/mysql"
)

func TestRequireURIUsesConfiguredURI(t *testing.T) {
	want := "mysql://root:secret@example.test:3306/common"
	t.Setenv("MYSQL_URI", want)
	if got := testmysql.RequireURI(t); got != want {
		t.Fatalf("RequireURI() = %q, want %q", got, want)
	}
}
