// Package commandsource adapts one dynamic Configuration document to a command Source.
package commandsource

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"xiaoshiai.cn/common/command"
	"xiaoshiai.cn/common/config"
	confighttp "xiaoshiai.cn/common/config/http"
	confignoop "xiaoshiai.cn/common/config/noop"
)

const (
	addressFlag        = "configcenter-address"
	tokenFlag          = "configcenter-token"
	addressEnvironment = "CONFIGCENTER_ADDRESS"
	tokenEnvironment   = "CONFIGCENTER_TOKEN"
)

var (
	_ command.Source           = Source{}
	_ command.GlobalFlagSource = ConfigcenterSource{}
)

// Options configures a Configcenter client.
type Options struct {
	Address string `json:"address" description:"Configuration center address including its API prefix"`
	Token   string `json:"token" config:"token,sensitive" description:"Bearer token for the configuration center"`
}

// NewDefaultOptions returns disabled Configcenter options.
func NewDefaultOptions() *Options {
	return &Options{}
}

// Source loads one Configuration through an injected DynamicConfig.
type Source struct {
	client    config.DynamicConfig
	namespace string
	name      string
}

// New returns a Source backed by one named Configuration.
func New(client config.DynamicConfig, namespace, name string) Source {
	return Source{client: client, namespace: namespace, name: name}
}

// Name identifies the Configcenter Source in diagnostics.
func (Source) Name() string {
	return "configcenter"
}

// Load selects the global or action section for the current target.
func (s Source) Load(ctx context.Context, input command.SourceInput) ([]command.SourceValue, error) {
	return load(ctx, s.client, s.namespace, s.name, input)
}

// ConfigcenterSource resolves its DynamicConfig from Configcenter control values.
type ConfigcenterSource struct {
	options   Options
	namespace string
	name      string
}

// FromOptions returns a Configcenter Source. Address defaults to empty, which
// selects the Noop adapter; a non-empty address without a scheme uses HTTP.
func FromOptions(options *Options, namespace, name string) ConfigcenterSource {
	return ConfigcenterSource{options: *options, namespace: namespace, name: name}
}

// DefaultSources returns the standard command Sources with Configuration
// center loading between files and environment variables.
func DefaultSources(namespace, name string) []command.Source {
	return []command.Source{
		command.ConfigurationFiles(),
		FromOptions(NewDefaultOptions(), namespace, name),
		command.EnvironmentVariables(),
		command.CommandLineArguments(),
	}
}

// Name identifies the Configcenter Source in diagnostics.
func (ConfigcenterSource) Name() string {
	return "configcenter"
}

// GlobalFlags returns the control flags needed before action selection.
func (ConfigcenterSource) GlobalFlags() []command.Flag {
	return []command.Flag{
		{
			Pattern:   addressFlag,
			ValueMode: command.RequiredFlagValue,
			ValueName: "address",
			Summary:   "Configcenter address",
		},
		{
			Pattern:   tokenFlag,
			ValueMode: command.RequiredFlagValue,
			ValueName: "token",
			Summary:   "Configcenter bearer token",
			Hidden:    true,
		},
	}
}

// Load resolves the selected adapter and loads the target Configuration.
func (s ConfigcenterSource) Load(ctx context.Context, input command.SourceInput) ([]command.SourceValue, error) {
	options := s.options
	if address, exists := input.Environment[addressEnvironment]; exists {
		options.Address = address
	}
	if token, exists := input.Environment[tokenEnvironment]; exists {
		options.Token = token
	}
	for _, flag := range input.Flags {
		switch flag.Name {
		case addressFlag:
			options.Address = flag.Value
		case tokenFlag:
			options.Token = flag.Value
		}
	}
	client, err := newDynamicConfig(ctx, options)
	if err != nil {
		return nil, err
	}
	return load(ctx, client, s.namespace, s.name, input)
}

func newDynamicConfig(ctx context.Context, options Options) (config.DynamicConfig, error) {
	if options.Address == "" {
		return confignoop.New(), nil
	}
	address := options.Address
	if !strings.Contains(address, "://") {
		address = "http://" + address
	}
	parsed, err := url.Parse(address)
	if err != nil {
		return nil, fmt.Errorf("parse configuration center address: %w", err)
	}
	switch parsed.Scheme {
	case "http", "https":
		return confighttp.New(ctx, parsed.String(), options.Token)
	default:
		return nil, fmt.Errorf("unsupported configuration center address scheme %q", parsed.Scheme)
	}
}

func load(ctx context.Context, client config.DynamicConfig, namespace, name string, input command.SourceInput) ([]command.SourceValue, error) {
	decoded := any(nil)
	configuration, err := client.Get(ctx, namespace, name, &decoded)
	if err != nil {
		return nil, fmt.Errorf("load configuration %q/%q: %w", namespace, name, err)
	}
	if configuration == nil {
		return nil, nil
	}
	current, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("configuration %q/%q root must be an object", namespace, name)
	}
	path := input.Target.CommandPath
	if input.Target.Global {
		path = []string{"global"}
	}
	for _, section := range path {
		value, exists := current[section]
		if !exists {
			return nil, nil
		}
		current, ok = value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("configuration %q/%q section %q must be an object", namespace, name, strings.Join(path, "."))
		}
	}
	if !input.Target.Global && len(input.Target.CommandPath) == 0 {
		delete(current, "global")
	}
	return []command.SourceValue{{Name: name, Value: current}}, nil
}
