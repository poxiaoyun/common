package sql

import (
	"testing"

	"xiaoshiai.cn/common/store"
)

func TestJSONNamingStrategy(t *testing.T) {
	namer := jsonNamingStrategy{}
	tests := map[string]string{
		"ID":                "id",
		"UID":               "uid",
		"APIVersion":        "apiVersion",
		"ResourceVersion":   "resourceVersion",
		"CreationTimestamp": "creationTimestamp",
	}
	for input, want := range tests {
		if got := namer.ColumnName("", input); got != want {
			t.Errorf("ColumnName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestBuildCreateIndexQuery(t *testing.T) {
	index := store.Index{Name: "email_tenant", Fields: []string{"email", "tenant"}, Unique: true, Nullable: true}
	if got, want := buildCreateIndexQuery(DBDriverPostgres, "users", index), `CREATE UNIQUE INDEX "email_tenant" ON "users" ("email", "tenant") WHERE "email" IS NOT NULL AND "tenant" IS NOT NULL`; got != want {
		t.Fatalf("postgres query = %q, want %q", got, want)
	}
	if got, want := buildCreateIndexQuery(DBDriverMySQL, "users", index), "CREATE UNIQUE INDEX `email_tenant` ON `users` (`email`, `tenant`)"; got != want {
		t.Fatalf("mysql query = %q, want %q", got, want)
	}
}

func TestBuildDropPrimaryKeyQuery(t *testing.T) {
	if got, want := buildDropPrimaryKeyQuery(DBDriverPostgres, "users", "users_pkey"), `ALTER TABLE "users" DROP CONSTRAINT "users_pkey"`; got != want {
		t.Fatalf("postgres query = %q, want %q", got, want)
	}
	if got, want := buildDropPrimaryKeyQuery(DBDriverMySQL, "users", "PRIMARY"), "ALTER TABLE `users` DROP PRIMARY KEY"; got != want {
		t.Fatalf("mysql query = %q, want %q", got, want)
	}
}
