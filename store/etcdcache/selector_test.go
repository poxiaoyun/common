package etcdcache

import (
	"testing"

	"k8s.io/apimachinery/pkg/fields"
	"xiaoshiai.cn/common/store"
)

func TestConvertPredicateSupportsFieldPresenceOperators(t *testing.T) {
	tests := []struct {
		name     string
		operator store.Operator
		values   fields.Set
		want     bool
	}{
		{name: "exists with field", operator: store.Exists, values: fields.Set{"organization": "acme"}, want: true},
		{name: "exists without field", operator: store.Exists, values: fields.Set{}, want: false},
		{name: "does not exist with field", operator: store.DoesNotExist, values: fields.Set{"organization": "acme"}, want: false},
		{name: "does not exist without field", operator: store.DoesNotExist, values: fields.Set{}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			predicate, err := ConvertPredicate(nil, store.Requirements{
				store.NewRequirement("organization", tt.operator),
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := predicate.Field.Matches(tt.values); got != tt.want {
				t.Fatalf("Field.Matches(%v) = %t, want %t", tt.values, got, tt.want)
			}
		})
	}
}
