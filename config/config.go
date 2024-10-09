package config

import (
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"k8s.io/component-base/cli/globalflag"
	"xiaoshiai.cn/common/log"
	libreflect "xiaoshiai.cn/common/reflect"
)

// Parse 从多个方式加载配置
/*
 * 配置文件加载有如下优先级：
 - 命令行参数
 - 环境变量
 - 默认值

- 高优先级的配置若存在，会覆盖低优先级已存在的配置
- 若所有配置均不存在，则使用默认值

对于需要做配置的项目，需要先设置 flag，环境和配置文件会使用已经设置flag进行配置

举例：
若需要增加配置项目,需要配置使用的结构并设置默认值，例：Foo{Bar:"默认值"},
然后使用 pflag 配置命令行参数：

	fs.StringVarP(&options.Foo.Bar, "foo-bar", "", options.Foo.Bar, "foo bar")

配置完成后,Parse 会根据 plagset 中已有配置 "foo-bar",获取对应的环境变量 "FOO_BAR"，以及对应的配置文件项 "foo.bar"
*/
func Parse(fs *pflag.FlagSet) error {
	// 从默认值配置
	// fs 中已有默认值
	printDefault(fs)
	// 从环境变量配置
	LoadEnv(fs)
	// 从命令行配置
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}
	// help
	if fs.Lookup("help").Value.String() == "true" {
		if fs.Usage != nil {
			fs.Usage()
		}
		os.Exit(0)
	}
	// print
	Print(fs)
	return nil
}

func Print(fs *pflag.FlagSet) {
	fs.VisitAll(func(flag *pflag.Flag) {
		if flag.Changed {
			logConfig("flag", "--"+flag.Name, flag.Value.String())
		}
	})
}

func RegisterFlags(fs *pflag.FlagSet, prefix string, data interface{}) {
	node := libreflect.ParseStruct(data)
	registerFlagSet(fs, prefix, node.Fields)

	// gloabal flags
	globalflag.AddGlobalFlags(fs, "")
}

func LoadEnv(fs *pflag.FlagSet) {
	flagNameToEnvKey := func(fname string) string {
		return strings.ToUpper(strings.ReplaceAll(fname, "-", "_"))
	}
	fs.VisitAll(func(f *pflag.Flag) {
		envname := flagNameToEnvKey(f.Name)
		val, ok := os.LookupEnv(envname)
		if ok {
			logConfig("env", envname, val)
			_ = f.Value.Set(val)
		}
	})
}

func flagNameToConfigKey(fname string) string {
	return strings.ToLower(strings.ReplaceAll(fname, "-", "."))
}

func printDefault(fs *pflag.FlagSet) {
	fs.VisitAll(func(f *pflag.Flag) {
		logConfig("default", flagNameToConfigKey(f.Name), f.DefValue)
	})
}

func logConfig(from, k, v string) {
	if strings.Contains(strings.ToLower(k), "password") {
		v = strings.Repeat("*", len(v))
	}
	log.Info("config", "from", from, "key", k, "val", v)
}

func registerFlagSet(fs *pflag.FlagSet, prefix string, nodes []libreflect.Node) {
	for _, node := range nodes {
		key := prefixedKey(prefix, node.Name)
		if node.Kind == reflect.Pointer && node.Value.IsNil() {
			log.Info("skip null config", "key", "--"+key)
			continue
		}
		switch node.Kind {
		case reflect.Struct, reflect.Map:
			registerFlagSet(fs, key, node.Fields)
		default:
			short := node.Tag.Get("short")
			description := node.Tag.Get("description")
			if !node.Value.CanAddr() {
				continue
			}
			v := node.Value.Addr().Interface()
			switch value := v.(type) {
			case *string:
				fs.StringVarP(value, key, short, *value, description)
			case *bool:
				fs.BoolVarP(value, key, short, *value, description)
			case *int:
				fs.IntVarP(value, key, short, *value, description)
			case *int64:
				fs.Int64VarP(value, key, short, *value, description)
			case *uint16:
				fs.Uint16VarP(value, key, short, *value, description)
			case *[]bool:
				fs.BoolSliceVarP(value, key, short, *value, description)
			case *time.Duration:
				fs.DurationVarP(value, key, short, *value, description)
			case *float32:
				fs.Float32VarP(value, key, short, *value, description)
			case *float64:
				fs.Float64VarP(value, key, short, *value, description)
			case *[]string:
				fs.StringSliceVarP(value, key, short, *value, description)
			default:
			}
		}
	}
}

func prefixedKey(prefix, key string) string {
	if len(prefix) == 0 {
		return strings.ToLower(key)
	}
	spl := "-"
	return strings.ToLower(prefix + spl + key)
}
