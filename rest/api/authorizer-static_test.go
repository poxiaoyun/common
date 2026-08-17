package api

import "testing"

func TestStaticAuthorizer(t *testing.T) {
	type args struct {
		rules          []StaticAuthorizationRule
		authentication AuthenticationInfo
		attributes     Attributes
	}
	tests := []struct {
		name         string
		args         args
		wantDecision Decision
		wantReason   string
		wantErr      bool
	}{
		{
			name: "matching rule allows",
			args: args{
				rules: []StaticAuthorizationRule{
					{
						Actions:   []string{"get"},
						Resources: []string{"clusters:*:metadata:*"},
					},
				},
				attributes: Attributes{
					Action: "get",
					Resources: []AttributeResource{
						{Resource: "clusters", Name: "default"},
						{Resource: "metadata", Name: "test"},
					},
				},
			},
			wantDecision: DecisionAllow,
		},
		{
			name: "matching collection rule allows",
			args: args{
				rules: []StaticAuthorizationRule{
					{
						Actions:   []string{"get", "list"},
						Resources: []string{"clusters:*:metadata"},
					},
				},
				attributes: Attributes{
					Action: "list",
					Resources: []AttributeResource{
						{Resource: "clusters", Name: "default"},
						{Resource: "metadata"},
					},
				},
			},
			wantDecision: DecisionAllow,
		},
		{
			name: "matching rule delegates",
			args: args{
				rules: []StaticAuthorizationRule{
					{
						Actions:    []string{"get"},
						Resources:  []string{"clusters:*:metadata:*"},
						Authorizer: NewAlwaysDenyAuthorizer(),
					},
				},
				attributes: Attributes{
					Action: "get",
					Resources: []AttributeResource{
						{Resource: "clusters", Name: "default"},
						{Resource: "metadata", Name: "test"},
					},
				},
			},
			wantDecision: DecisionDeny,
		},
		{
			name: "unmatched rule has no opinion",
			args: args{
				rules: []StaticAuthorizationRule{
					{
						Actions:   []string{"get"},
						Resources: []string{"clusters:*:metadata:*"},
					},
				},
				attributes: Attributes{
					Action: "list",
					Resources: []AttributeResource{
						{Resource: "clusters", Name: "default"},
						{Resource: "metadata"},
					},
				},
			},
			wantDecision: DecisionNoOpinion,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorizer := NewStaticAuthorizer(tt.args.rules)
			decision, reason, err := authorizer.Authorize(t.Context(), tt.args.authentication, tt.args.attributes)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Authorize() error = %v, want error %t", err, tt.wantErr)
			}
			if decision != tt.wantDecision {
				t.Errorf("Authorize() decision = %v, want %v", decision, tt.wantDecision)
			}
			if reason != tt.wantReason {
				t.Errorf("Authorize() reason = %v, want %v", reason, tt.wantReason)
			}
		})
	}
}
