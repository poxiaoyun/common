package controller

import (
	"testing"

	"xiaoshiai.cn/common/store"
)

func Test_decodeScopes(t *testing.T) {
	tests := [][]store.Scope{
		{
			{Resource: "foo", Name: "bar"},
			{Resource: "a", Name: "b"},
		},
		{},
		{
			{Name: "bar"},
		},
		{
			{Name: "bar"},
			{Name: "bar"},
		},
	}

	for _, tt := range tests {
		encoded := encodeScopes(tt)
		decoded := decodeScopes(encoded)
		if len(tt) != len(decoded) {
			t.Errorf("decodeScopes() = %v, want %v", decoded, tt)
		}
	}
}
