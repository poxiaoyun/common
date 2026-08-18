package command_test

import (
	"context"
	"fmt"
	"io/fs"
	"os"

	"xiaoshiai.cn/common/command"
)

func ExampleExec() {
	type options struct {
		Listen string `config:"listen"`
		Debug  bool   `config:"debug"`
	}
	program := command.Program{Command: command.Command{
		Name: "tool",
		Children: []command.Command{{
			Name:      "serve",
			Arguments: []command.Argument{{Name: "arguments", Optional: true, Variadic: true}},
			Options:   func() any { return &options{Listen: ":8080"} },
			Run: func(invocation command.Invocation) error {
				configured := command.Options[options](invocation)
				fmt.Fprintf(invocation.Streams.Output, "listen=%s debug=%t arguments=%v\n", configured.Listen, configured.Debug, invocation.Arguments)
				return nil
			},
		}},
	}}
	err := command.Exec(context.Background(), program, command.Execution{
		Arguments: []string{
			"--config-file=config.yaml",
			"serve",
			"input.txt",
			"--listen=:9090",
			"--",
			"--literal",
			"-1",
		},
		Environment: map[string]string{"DEBUG": "true"},
		ReadFile: func(name string) ([]byte, error) {
			if name == "config.yaml" {
				return []byte("serve:\n  listen: :7000\n"), nil
			}
			return nil, fs.ErrNotExist
		},
		Streams: command.Streams{Output: os.Stdout},
	})
	if err != nil {
		fmt.Println("error:", err)
	}

	// Output:
	// listen=:9090 debug=true arguments=[input.txt --literal -1]
}
