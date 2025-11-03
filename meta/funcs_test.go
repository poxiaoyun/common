package meta_test

import (
	"reflect"
	"testing"
	"time"

	"xiaoshiai.cn/common/meta"
)

func TestParseString(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		got := meta.ParseString("hello", "")
		if got != "hello" {
			t.Fatalf("expected %q, got %q", "hello", got)
		}
	})

	t.Run("[]string trim and split", func(t *testing.T) {
		got := meta.ParseString[[]string]("a, b ,c", nil)
		want := []string{"a", "b", "c"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("expected %#v, got %#v", want, got)
		}
	})

	t.Run("int", func(t *testing.T) {
		got := meta.ParseString("123", 0)
		if got != 123 {
			t.Fatalf("expected %d, got %d", 123, got)
		}
	})

	t.Run("bool", func(t *testing.T) {
		got := meta.ParseString("true", false)
		if got != true {
			t.Fatalf("expected true, got %v", got)
		}
	})

	t.Run("time RFC3339", func(t *testing.T) {
		want, _ := time.Parse(time.RFC3339, "2023-10-01T12:00:00Z")
		got := meta.ParseString("2023-10-01T12:00:00Z", time.Time{})
		if !got.Equal(want) {
			t.Fatalf("expected %v, got %v", want, got)
		}
	})

	t.Run("pointer int", func(t *testing.T) {
		got := meta.ParseString[*int]("42", nil)
		if got == nil || *got != 42 {
			t.Fatalf("expected *int 42, got %#v", got)
		}
	})
}
