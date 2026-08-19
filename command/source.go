package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"

	"sigs.k8s.io/yaml"
	libreflect "xiaoshiai.cn/common/reflect"
)

// Source loads ordered configuration values for one configured target.
// Implementations must be reusable and safe for concurrent Program executions.
type Source interface {
	// Name identifies the source in diagnostics.
	Name() string
	// Load returns values in low-to-high priority order.
	Load(ctx context.Context, input SourceInput) ([]SourceValue, error)
}

// FlagSource is the optional capability implemented by Sources that own
// command-line flags.
type FlagSource interface {
	Source
	// Flags returns the long-option declarations derived from the current
	// semantic configuration tree and owned by this Source.
	Flags(configuration *libreflect.Node) ([]Flag, error)
}

// GlobalFlagSource is the optional capability implemented by Sources whose
// control flags must be recognized before action selection.
type GlobalFlagSource interface {
	Source
	GlobalFlags() []Flag
}

// Target identifies the configured action selected for one execution.
type Target struct {
	Executable  string
	CommandPath []string
	// Global identifies the Program's global-options target. Its CommandPath is
	// empty and configuration files use the top-level global section.
	Global bool
}

// SourceValue is one named configuration value loaded by a Source.
type SourceValue struct {
	// Name identifies the input in errors and configuration logs, such as a
	// file, environment variable, or command-line option.
	Name  string
	Value any
}

// SourceInput contains the immutable execution inputs available to every
// Source.
type SourceInput struct {
	Target        Target
	Arguments     []string
	Environment   map[string]string
	ReadFile      func(string) ([]byte, error)
	Flags         []FlagValue
	Configuration *libreflect.Node
}

// FlagValueMode declares how a long option obtains its value.
type FlagValueMode uint8

const (
	// RequiredFlagValue requires either --name=value or --name value.
	RequiredFlagValue FlagValueMode = iota
	// BooleanFlagValue also accepts bare --name as true.
	BooleanFlagValue
)

// Flag declares one long option. Pattern is an exact name without leading
// dashes or contains one bracketed map-key placeholder, such as labels[{key}].
type Flag struct {
	Pattern      string
	Short        string
	ValueMode    FlagValueMode
	ValueName    string
	Summary      string
	DefaultValue string
	ShowDefault  bool
	Hidden       bool
}

// FlagValue is one occurrence matched to a Flag owned by the current Source.
type FlagValue struct {
	Name     string
	Value    string
	Position int
}

// Compiled flags stay private because matching and ownership belong to Exec,
// not the Source extension point.
type compiledFlag struct {
	Flag
	sourceIndex int
	prefix      string
	suffix      string
	dynamic     bool
}

func compileSourceFlags(sources []Source, configuration *libreflect.Node) ([]compiledFlag, error) {
	flags := []compiledFlag{}
	for sourceIndex, source := range sources {
		owner, exists := source.(FlagSource)
		if !exists {
			continue
		}
		declared, err := owner.Flags(configuration)
		if err != nil {
			return nil, fmt.Errorf("source %q: %w", source.Name(), err)
		}
		for _, declaration := range declared {
			compiled, err := compileFlag(declaration)
			if err != nil {
				return nil, fmt.Errorf("source %q: %w", source.Name(), err)
			}
			compiled.sourceIndex = sourceIndex
			for _, existing := range flags {
				if flagsOverlap(existing, compiled) {
					return nil, fmt.Errorf(
						"sources %q and %q declare overlapping flags --%s and --%s",
						sources[existing.sourceIndex].Name(), source.Name(), existing.Pattern, compiled.Pattern,
					)
				}
			}
			flags = append(flags, compiled)
		}
	}
	return flags, nil
}

func compileFlag(flag Flag) (compiledFlag, error) {
	if flag.Pattern == "" || strings.HasPrefix(flag.Pattern, "-") || strings.ContainsAny(flag.Pattern, "= \t\r\n") {
		return compiledFlag{}, fmt.Errorf("invalid flag pattern %q", flag.Pattern)
	}
	if flag.Short != "" && (len([]rune(flag.Short)) != 1 || strings.ContainsAny(flag.Short, "-= \t\r\n")) {
		return compiledFlag{}, fmt.Errorf("flag --%s has invalid short name %q", flag.Pattern, flag.Short)
	}
	switch flag.ValueMode {
	case RequiredFlagValue, BooleanFlagValue:
	default:
		return compiledFlag{}, fmt.Errorf("flag --%s has invalid value mode %d", flag.Pattern, flag.ValueMode)
	}
	compiled := compiledFlag{Flag: flag}
	const marker = "[{key}]"
	count := strings.Count(flag.Pattern, marker)
	if count > 1 || count == 0 && strings.Contains(flag.Pattern, "{key}") {
		return compiledFlag{}, fmt.Errorf("invalid flag pattern %q", flag.Pattern)
	}
	if count == 1 {
		compiled.dynamic = true
		compiled.prefix, compiled.suffix, _ = strings.Cut(flag.Pattern, marker)
		if compiled.prefix == "" {
			return compiledFlag{}, fmt.Errorf("invalid flag pattern %q", flag.Pattern)
		}
	}
	return compiled, nil
}

func flagsOverlap(left, right compiledFlag) bool {
	if left.Short != "" && left.Short == right.Short {
		return true
	}
	if !left.dynamic && !right.dynamic {
		return left.Pattern == right.Pattern
	}
	if !left.dynamic {
		return right.match(left.Pattern)
	}
	if !right.dynamic {
		return left.match(right.Pattern)
	}
	return left.prefix == right.prefix && left.suffix == right.suffix
}

func (flag compiledFlag) match(name string) bool {
	if !flag.dynamic {
		return name == flag.Pattern
	}
	if !strings.HasPrefix(name, flag.prefix+"[") || !strings.HasSuffix(name, "]"+flag.suffix) {
		return false
	}
	key := strings.TrimSuffix(strings.TrimPrefix(name, flag.prefix+"["), "]"+flag.suffix)
	return key != "" && !strings.ContainsRune(key, ']')
}

// DefaultSources returns fresh standard Sources in low-to-high priority order.
func DefaultSources() []Source {
	return []Source{ConfigurationFiles(), EnvironmentVariables(), CommandLineArguments()}
}

// ConfigurationFiles returns the YAML and JSON configuration-file Source.
func ConfigurationFiles() ConfigurationFilesSource {
	return ConfigurationFilesSource{}
}

// EnvironmentVariables returns the environment-variable Source.
func EnvironmentVariables() EnvironmentVariablesSource {
	return EnvironmentVariablesSource{}
}

// CommandLineArguments returns the typed command-line configuration Source.
func CommandLineArguments() CommandLineArgumentsSource {
	return CommandLineArgumentsSource{}
}

// ConfigurationFilesSource loads YAML and JSON configuration files.
type ConfigurationFilesSource struct{}

// Name identifies the configuration-file source in diagnostics.
func (ConfigurationFilesSource) Name() string {
	return "configuration-files"
}

// GlobalFlags declares the option used to select an explicit configuration file.
func (ConfigurationFilesSource) GlobalFlags() []Flag {
	return []Flag{{
		Pattern:   "config-file",
		ValueMode: RequiredFlagValue,
		ValueName: "string",
		Summary:   "Configuration file (.yaml or .json)",
	}}
}

// Load returns discovered or explicitly selected configuration files.
func (ConfigurationFilesSource) Load(_ context.Context, input SourceInput) ([]SourceValue, error) {
	selected := ""
	for _, flag := range input.Flags {
		if flag.Name == "config-file" {
			selected = flag.Value
		}
	}
	if selected == "" {
		selected = input.Environment["CONFIG_FILE"]
	}
	paths := []string{
		"config/" + input.Target.Executable + ".yaml",
		"config/" + input.Target.Executable + ".json",
		input.Target.Executable + ".yaml",
		input.Target.Executable + ".json",
	}
	explicit := selected != ""
	if explicit {
		if extension := strings.ToLower(filepath.Ext(selected)); extension != ".yaml" && extension != ".json" {
			return nil, fmt.Errorf("configuration file %q must use .yaml or .json", selected)
		}
		paths = []string{selected}
	}
	values := []SourceValue{}
	for _, path := range paths {
		data, err := input.ReadFile(path)
		if err != nil {
			if !explicit && errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		commandPath := input.Target.CommandPath
		if input.Target.Global {
			commandPath = []string{"global"}
		}
		decoded, err := decodeConfigurationFile(data, path, commandPath)
		if err != nil {
			return nil, fmt.Errorf("decode %s: %w", path, err)
		}
		if !input.Target.Global && len(input.Target.CommandPath) == 0 {
			delete(decoded, "global")
		}
		values = append(values, SourceValue{Name: path, Value: decoded})
	}
	return values, nil
}

// EnvironmentVariablesSource loads typed options from environment variables.
type EnvironmentVariablesSource struct{}

// Name identifies the environment-variable source in diagnostics.
func (EnvironmentVariablesSource) Name() string {
	return "environment"
}

// Load returns recognized environment variables as configuration values.
func (EnvironmentVariablesSource) Load(_ context.Context, input SourceInput) ([]SourceValue, error) {
	type environmentProperty struct {
		name     string
		property string
		value    string
	}
	properties := []environmentProperty{}
	fixed := map[string]struct{}{}
	for _, field := range projectConfigurationFields(input.Configuration, "", nil, false, nil) {
		environment := configurationEnvironmentName(field.Canonical)
		fixed[environment] = struct{}{}
		if raw, exists := input.Environment[environment]; exists {
			properties = append(properties, environmentProperty{name: environment, property: field.Canonical, value: raw})
		}
	}
	for name, raw := range input.Environment {
		if _, exists := fixed[name]; exists || name == "CONFIG_FILE" {
			continue
		}
		property, exists := resolveMapEnvironment(input.Configuration, name)
		if exists {
			properties = append(properties, environmentProperty{name: name, property: property, value: raw})
		}
	}
	sort.Slice(properties, func(i, j int) bool {
		left, right := propertyDepth(properties[i].property), propertyDepth(properties[j].property)
		if left != right {
			return left < right
		}
		return properties[i].property < properties[j].property
	})
	values := make([]SourceValue, 0, len(properties))
	for _, property := range properties {
		parts, err := parsePropertyPath(property.property)
		if err != nil {
			return nil, err
		}
		values = append(values, SourceValue{Name: property.name, Value: configurationPatch(parts, property.value)})
	}
	return values, nil
}

// CommandLineArgumentsSource loads typed options from command-line flags.
type CommandLineArgumentsSource struct{}

// Name identifies the command-line source in diagnostics.
func (CommandLineArgumentsSource) Name() string {
	return "command-line"
}

// Flags derives typed option declarations from the configured action options.
func (CommandLineArgumentsSource) Flags(configuration *libreflect.Node) ([]Flag, error) {
	flags := []Flag{}
	for _, field := range projectConfigurationFields(configuration, "", nil, false, nil) {
		flag, err := configurationFlag(field)
		if err != nil {
			return nil, fmt.Errorf("configuration property %q default: %w", field.Canonical, err)
		}
		flags = append(flags, flag)
		if field.Node.Kind != libreflect.ObjectNode || field.Node.Element == nil {
			continue
		}
		if field.Node.Element.Kind == libreflect.ObjectNode && field.Node.Element.Element == nil {
			for _, child := range projectConfigurationFields(field.Node.Element, "", nil, field.Sensitive, nil) {
				flags = append(flags, Flag{
					Pattern:   configurationFlagName(field.Canonical) + "[{key}]-" + configurationFlagName(child.Canonical),
					ValueMode: flagValueMode(child.Node.Type),
					ValueName: configurationTypeName(child.Node.Type),
					Hidden:    true,
				})
			}
			continue
		}
		flags = append(flags, Flag{
			Pattern:   configurationFlagName(field.Canonical) + "[{key}]",
			ValueMode: flagValueMode(field.Node.Element.Type),
			ValueName: configurationTypeName(field.Node.Element.Type),
			Hidden:    true,
		})
	}
	return flags, nil
}

func configurationFlag(field projectedConfigurationField) (Flag, error) {
	short, err := configurationShortName(field.Node.Tag.Get("config"))
	if err != nil {
		return Flag{}, err
	}
	flag := Flag{
		Pattern:   configurationFlagName(field.Canonical),
		Short:     short,
		ValueMode: flagValueMode(field.Node.Type),
		ValueName: configurationTypeName(field.Node.Type),
		Summary:   field.Node.Tag.Get("description"),
	}
	if !field.Sensitive {
		value := any(nil)
		if configurationStringDecoder(field.Node.Type) {
			value = field.Node.Value.Interface()
		} else {
			value = encodeConfigurationNode(field.Node, field.Node.Value)
		}
		if !isZeroConfigurationData(value) {
			var err error
			flag.DefaultValue, err = libreflect.FormatValue(value)
			if err != nil {
				return Flag{}, err
			}
			flag.ShowDefault = true
		}
	}
	return flag, nil
}

func configurationShortName(tag string) (string, error) {
	short := ""
	for _, option := range strings.Split(tag, ",")[1:] {
		value, found := strings.CutPrefix(option, "short=")
		if !found {
			continue
		}
		if short != "" {
			return "", fmt.Errorf("multiple short names")
		}
		short = value
	}
	return short, nil
}

func configurationFlagName(canonical string) string {
	return strings.ToLower(strings.ReplaceAll(canonical, ".", "-"))
}

func flagValueMode(typ reflect.Type) FlagValueMode {
	if libreflect.IndirectType(typ).Kind() == reflect.Bool {
		return BooleanFlagValue
	}
	return RequiredFlagValue
}

// Load returns matched command-line flags as configuration values.
func (CommandLineArgumentsSource) Load(_ context.Context, input SourceInput) ([]SourceValue, error) {
	sort.SliceStable(input.Flags, func(i, j int) bool { return input.Flags[i].Position < input.Flags[j].Position })
	values := make([]SourceValue, 0, len(input.Flags))
	for _, flag := range input.Flags {
		_, property, exists := resolveCommandLineProperty(input.Configuration, flag.Name)
		if !exists {
			return nil, fmt.Errorf("unknown option --%s", flag.Name)
		}
		parts, err := parsePropertyPath(property)
		if err != nil {
			return nil, err
		}
		values = append(values, SourceValue{Name: "--" + flag.Name, Value: configurationPatch(parts, flag.Value)})
	}
	return values, nil
}

func resolveCommandLineProperty(schema *libreflect.Node, name string) (*libreflect.Node, string, bool) {
	fields := projectConfigurationFields(schema, "", nil, false, nil)
	for _, field := range fields {
		if configurationFlagName(field.Canonical) == name {
			return field.Node, field.Canonical, true
		}
	}
	open := strings.IndexByte(name, '[')
	close := strings.LastIndexByte(name, ']')
	if open <= 0 || close <= open || close != len(name)-1 && name[close+1] != '-' {
		return nil, "", false
	}
	base, key := name[:open], name[open+1:close]
	var field projectedConfigurationField
	found := false
	for _, candidate := range fields {
		if configurationFlagName(candidate.Canonical) == base {
			field = candidate
			found = true
			break
		}
	}
	if !found || field.Node.Kind != libreflect.ObjectNode || field.Node.Element == nil || key == "" || strings.ContainsRune(key, ']') {
		return nil, "", false
	}
	property := field.Canonical + "[" + key + "]"
	if close == len(name)-1 {
		return field.Node.Element, property, true
	}
	suffix := name[close+2:]
	child, canonical, exists := resolveRelativeFlag(field.Node.Element, suffix)
	if !exists {
		return nil, "", false
	}
	return child, property + "." + canonical, true
}

func resolveRelativeFlag(node *libreflect.Node, flag string) (*libreflect.Node, string, bool) {
	if node.Kind != libreflect.ObjectNode || node.Element != nil {
		return nil, "", false
	}
	for _, field := range projectConfigurationFields(node, "", nil, false, nil) {
		if configurationFlagName(field.Canonical) == flag {
			return field.Node, field.Canonical, true
		}
	}
	return nil, "", false
}

func resolveMapEnvironment(schema *libreflect.Node, name string) (string, bool) {
	maps := []projectedConfigurationField{}
	for _, field := range projectConfigurationFields(schema, "", nil, false, nil) {
		if field.Node.Kind == libreflect.ObjectNode && field.Node.Element != nil {
			maps = append(maps, field)
		}
	}
	sort.Slice(maps, func(i, j int) bool {
		return len(configurationEnvironmentName(maps[i].Canonical)) > len(configurationEnvironmentName(maps[j].Canonical))
	})
	for _, field := range maps {
		environment := configurationEnvironmentName(field.Canonical)
		prefix := environment + "_"
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		remainder := strings.TrimPrefix(name, prefix)
		if remainder == "" {
			continue
		}
		if field.Node.Element.Kind != libreflect.ObjectNode || field.Node.Element.Element != nil {
			return field.Canonical + "[" + strings.ToLower(remainder) + "]", true
		}
		children := projectConfigurationFields(field.Node.Element, "", nil, field.Sensitive, nil)
		sort.Slice(children, func(i, j int) bool {
			return len(configurationEnvironmentName(children[i].Canonical)) > len(configurationEnvironmentName(children[j].Canonical))
		})
		for _, child := range children {
			suffix := "_" + configurationEnvironmentName(child.Canonical)
			if !strings.HasSuffix(remainder, suffix) || len(remainder) == len(suffix) {
				continue
			}
			key := strings.ToLower(strings.TrimSuffix(remainder, suffix))
			return field.Canonical + "[" + key + "]." + child.Canonical, true
		}
	}
	return "", false
}

func configurationEnvironmentName(canonical string) string {
	return strings.ToUpper(strings.NewReplacer(".", "_", "-", "_").Replace(canonical))
}

type propertyPart struct {
	Name   string
	MapKey bool
}

func parsePropertyPath(property string) ([]propertyPart, error) {
	if property == "" {
		return nil, fmt.Errorf("configuration property name is empty")
	}
	parts := []propertyPart{}
	for index := 0; index < len(property); {
		start := index
		for index < len(property) && property[index] != '.' && property[index] != '[' {
			index++
		}
		if index > start {
			parts = append(parts, propertyPart{Name: property[start:index]})
		}
		if index == len(property) {
			break
		}
		switch property[index] {
		case '.':
			index++
			if index == len(property) || property[index] == '.' || property[index] == '[' {
				return nil, fmt.Errorf("invalid configuration property %q", property)
			}
		case '[':
			close := strings.IndexByte(property[index+1:], ']')
			if close < 0 {
				return nil, fmt.Errorf("invalid configuration property %q", property)
			}
			close += index + 1
			key := property[index+1 : close]
			if key == "" {
				return nil, fmt.Errorf("invalid configuration property %q", property)
			}
			parts = append(parts, propertyPart{Name: key, MapKey: true})
			index = close + 1
			if index < len(property) && property[index] == '.' {
				index++
			}
		}
	}
	return parts, nil
}

func configurationChild(node *libreflect.Node, name string) *libreflect.Node {
	for index := range node.Fields {
		if node.Fields[index].Name == name {
			return &node.Fields[index]
		}
	}
	return nil
}

func configurationPatch(parts []propertyPart, raw any) map[string]any {
	value := raw
	for _, part := range slices.Backward(parts) {
		value = map[string]any{part.Name: value}
	}
	return value.(map[string]any)
}

func propertyDepth(property string) int {
	return strings.Count(property, ".") + strings.Count(property, "[")
}

func joinCanonical(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

func decodeConfigurationFile(data []byte, path string, commandPath []string) (map[string]any, error) {
	extension := strings.ToLower(filepath.Ext(path))
	if extension == ".json" && !json.Valid(data) {
		return nil, fmt.Errorf("invalid JSON configuration")
	}
	jsonData, err := yaml.YAMLToJSONStrict(data)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(jsonData)))
	decoder.UseNumber()
	decoded := any(nil)
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	root, ok := stringMap(decoded)
	if !ok {
		return nil, fmt.Errorf("configuration root must be an object")
	}
	current := root
	for _, command := range commandPath {
		value, exists := current[command]
		if !exists {
			return map[string]any{}, nil
		}
		next, ok := stringMap(value)
		if !ok {
			return nil, fmt.Errorf("command section %q must be an object", strings.Join(commandPath, "."))
		}
		current = next
	}
	return current, nil
}

func stringMap(value any) (map[string]any, bool) {
	object, ok := value.(map[string]any)
	return object, ok
}
