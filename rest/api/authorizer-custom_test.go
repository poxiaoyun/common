package api

import (
	"context"
	"testing"
)

func Test_checkRecursive(t *testing.T) {
	type args struct {
		allows []CustomAuthorizeNode
		user   UserInfo
		attr   Attributes
	}
	tests := []struct {
		name           string
		args           args
		wantAuthorized Decision
		wantReason     string
		wantErr        bool
	}{
		{
			args: args{
				allows: []CustomAuthorizeNode{
					{
						Actions:  []string{"get"},
						Resource: []string{"clusters:*:metadata:*"},
					},
				},
				attr: Attributes{
					Action: "get",
					Resources: []AttrbuteResource{
						{Resource: "clusters", Name: "default"},
						{Resource: "metadata", Name: "test"},
					},
				},
			},
			wantAuthorized: DecisionAllow,
		},
		{
			args: args{
				allows: []CustomAuthorizeNode{
					{
						Actions:  []string{"get", "list"},
						Resource: []string{"clusters:*:metadata"},
					},
				},
				attr: Attributes{
					Action: "list",
					Resources: []AttrbuteResource{
						{Resource: "clusters", Name: "default"},
						{Resource: "metadata"},
					},
				},
			},
			wantAuthorized: DecisionAllow,
		},
		{
			args: args{
				allows: []CustomAuthorizeNode{
					{
						Actions:    []string{"get"},
						Resource:   []string{"clusters:*:metadata:*"},
						Authorizer: NewAlwaysDenyAuthorizer(),
					},
				},
				attr: Attributes{
					Action: "get",
					Resources: []AttrbuteResource{
						{Resource: "clusters", Name: "default"},
						{Resource: "metadata", Name: "test"},
					},
				},
			},
			wantAuthorized: DecisionDeny,
		},
		{
			name: "not matched is no opinion",
			args: args{
				allows: []CustomAuthorizeNode{
					{
						Actions:  []string{"get"},
						Resource: []string{"clusters:*:metadata:*"},
					},
				},
				attr: Attributes{
					Action: "list",
					Resources: []AttrbuteResource{
						{Resource: "clusters", Name: "default"},
						{Resource: "metadata"},
					},
				},
			},
			wantAuthorized: DecisionNoOpinion,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorizer := NewCustomAuthorizer(tt.args.allows)
			gotAuthorized, gotReason, err := authorizer.Authorize(context.Background(), tt.args.user, tt.args.attr)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkRecursive() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotAuthorized != tt.wantAuthorized {
				t.Errorf("checkRecursive() gotAuthorized = %v, want %v", gotAuthorized, tt.wantAuthorized)
			}
			if gotReason != tt.wantReason {
				t.Errorf("checkRecursive() gotReason = %v, want %v", gotReason, tt.wantReason)
			}
		})
	}
}
