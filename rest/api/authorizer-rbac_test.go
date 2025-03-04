package api

import (
	"testing"
)

func TestMatchAttributes(t *testing.T) {
	type args struct {
		act string
		res string
		att Attributes
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "get namespaces:*",
			args: args{
				act: "get", res: "namespaces:*",
				att: Attributes{Action: "get", Resources: []AttrbuteResource{{Resource: "namespaces", Name: "default"}}},
			},
			want: true,
		},
		{
			name: "get namespaces",
			args: args{
				act: "get", res: "namespaces",
				att: Attributes{Action: "get", Resources: []AttrbuteResource{{Resource: "namespaces", Name: "default"}}},
			},
			want: false,
		},
		{
			name: "list namespaces",
			args: args{
				act: "list", res: "namespaces",
				att: Attributes{Action: "list", Resources: []AttrbuteResource{{Resource: "namespaces"}}},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchAttributes(tt.args.act, tt.args.res, tt.args.att); got != tt.want {
				t.Errorf("MatchAttributes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResourcesToWildcard(t *testing.T) {
	tests := []struct {
		resources []AttrbuteResource
		want      string
	}{
		{
			resources: []AttrbuteResource{{Resource: "namespaces", Name: "default"}},
			want:      "namespaces:default",
		},
		{
			resources: []AttrbuteResource{{Resource: "namespaces"}},
			want:      "namespaces",
		},
		{
			resources: []AttrbuteResource{
				{Resource: "namespaces", Name: "default"},
				{Resource: "pods", Name: "nginx-xxx"},
			},
			want: "namespaces:default:pods:nginx-xxx",
		},
		{
			resources: []AttrbuteResource{
				{Resource: "namespaces", Name: ""},
				{Resource: "pods", Name: "nginx-xxx"},
			},
			want: "namespaces::pods:nginx-xxx",
		},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := ResourcesToWildcard(tt.resources); got != tt.want {
				t.Errorf("ResourcesToWildcard() = %v, want %v", got, tt.want)
			}
		})
	}
}
