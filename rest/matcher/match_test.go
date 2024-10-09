// Copyright 2022 The kubegems.io Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package matcher

import (
	"reflect"
	"regexp"
	"testing"
)

func TestCompileSection(t *testing.T) {
	tests := []struct {
		name    string
		want    []Section
		wantErr bool
	}{
		{
			name: "/assets/prefix-*.css",
			want: []Section{
				{{Pattern: "/assets"}},
				{
					{Pattern: "/prefix-", Greedy: true},
					{Pattern: ".css"},
				},
			},
		},
		{
			name: "/zoo/tom",
			want: []Section{
				{{Pattern: "/zoo"}},
				{{Pattern: "/tom"}},
			},
		},
		{
			name: "/v1/proxy*",
			want: []Section{
				{{Pattern: "/v1"}},
				{{Pattern: "/proxy", Greedy: true}},
			},
		},

		{
			name: "/api/v{version}/{name}*",
			want: []Section{
				{{Pattern: "/api"}},
				{{Pattern: "/v"}, {Pattern: "{version}", VarName: "version"}},
				{{Pattern: "/"}, {Pattern: "{name}", VarName: "name", Greedy: true}},
			},
		},
		{
			name: "/{repository:(?:[a-zA-Z0-9]+(?:[._-][a-zA-Z0-9]+)*/?)+}*/manifests/{reference}",
			want: []Section{
				{
					{Pattern: "/"},
					{
						Pattern: "{repository:(?:[a-zA-Z0-9]+(?:[._-][a-zA-Z0-9]+)*/?)+}", VarName: "repository", Greedy: true,
						Validate: regexp.MustCompile(`^(?:[a-zA-Z0-9]+(?:[._-][a-zA-Z0-9]+)*/?)+$`),
					},
					{Pattern: "/manifests"},
					{Pattern: "/"},
					{Pattern: "{reference}", VarName: "reference"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := compileSections(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("Compile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Compile() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSection_score(t *testing.T) {
	tests := []struct {
		a  string
		b  string
		eq int
	}{
		{a: "a", b: "{a}", eq: 1},
		{a: "api", b: "{a}", eq: 1},
		{a: "v{a}*", b: "{a}", eq: 1},
		{a: "{a}*", b: "{a}*:action", eq: -1},
		{a: "/{a}*", b: "/{a}*/foo/{b}", eq: -1},
	}
	for _, tt := range tests {
		t.Run(tt.a, func(t *testing.T) {
			seca, _ := compile(tt.a)
			secb, _ := compile(tt.b)

			scorea, scoreb := seca.score(), secb.score()
			if (scorea == scoreb && tt.eq != 0) ||
				(scorea > scoreb && tt.eq != 1) ||
				(scorea < scoreb && tt.eq != -1) {
				t.Errorf("Section.score() a = %v, b= %v, want %v", scorea, scoreb, tt.eq)
			}
		})
	}
}

func TestCompileError_Error(t *testing.T) {
	tests := []struct {
		name   string
		fields CompileError
		want   string
	}{
		{
			fields: CompileError{
				Pattern:  "pre{name}suf",
				Position: 1,
				Str:      "pre",
				Message:  "invalid character",
			},
			want: "invalid [pre] in [pre{name}suf] at position 1: invalid character",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := tt.fields
			if got := e.Error(); got != tt.want {
				t.Errorf("CompileError.Error() = %v, want %v", got, tt.want)
			}
		})
	}
}
