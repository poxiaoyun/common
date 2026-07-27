package config

import (
	"testing"

	"github.com/spf13/pflag"
)

func TestParsePrecedence(t *testing.T) {
	t.Setenv("PARSE_TEST_VALUE", "environment")

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "environment overrides default", want: "environment"},
		{name: "flag overrides environment", args: []string{"--parse-test-value=flag"}, want: "flag"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := "default"
			flags := pflag.NewFlagSet(tt.name, pflag.ContinueOnError)
			flags.StringVar(&value, "parse-test-value", value, "")
			if err := Parse(flags, tt.args); err != nil {
				t.Fatal(err)
			}
			if value != tt.want {
				t.Fatalf("value = %q, want %q", value, tt.want)
			}
		})
	}
}

func TestParseLeavesHelpToCaller(t *testing.T) {
	flags := pflag.NewFlagSet("help", pflag.ContinueOnError)
	help := false
	flags.BoolVarP(&help, "help", "h", false, "")
	if err := Parse(flags, []string{"--help"}); err != nil {
		t.Fatal(err)
	}
	if !help {
		t.Fatal("help flag was not parsed")
	}
}
