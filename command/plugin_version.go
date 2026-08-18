package command

import (
	"context"
	"encoding/json"
	"io"

	"xiaoshiai.cn/common/version"
)

// Version returns the build-version Plugin.
func Version() VersionPlugin {
	return VersionPlugin{}
}

// VersionPlugin provides both --version and the version command.
type VersionPlugin struct{}

func (VersionPlugin) Name() string {
	return "version"
}

func (VersionPlugin) Commands() []Command {
	return []Command{{
		Name:    "version",
		Summary: "Print version information",
		Run: func(invocation Invocation) error {
			return writeVersion(invocation.Streams.Output)
		},
	}}
}

func (VersionPlugin) Flags() []GlobalFlag {
	return []GlobalFlag{{
		Flag: Flag{
			Pattern:   "version",
			ValueMode: BooleanFlagValue,
			Summary:   "Print version information",
		},
		StopRouting: true,
	}}
}

func (VersionPlugin) Handle(ctx context.Context, invocation PluginInvocation) (context.Context, bool, error) {
	if len(invocation.Values) == 0 {
		return ctx, false, nil
	}
	return ctx, true, writeVersion(invocation.Streams.Output)
}

func writeVersion(output io.Writer) error {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(version.Get())
}
