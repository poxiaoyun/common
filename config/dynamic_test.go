package config_test

import (
	"testing"

	"xiaoshiai.cn/common/config"
)

type writeOptionFunc func(*config.WriteOptions)

func (option writeOptionFunc) ApplyToWrite(options *config.WriteOptions) {
	option(options)
}

func TestWriteOptionsAreOpenAndValidated(t *testing.T) {
	resolved, err := config.ResolveWriteOptions(writeOptionFunc(func(options *config.WriteOptions) {
		version := int64(7)
		options.ExpectedVersion = &version
	}))
	if err != nil || resolved.ExpectedVersion == nil || *resolved.ExpectedVersion != 7 {
		t.Fatalf("ResolveWriteOptions() = %#v, %v", resolved, err)
	}
	if _, err := config.ResolveWriteOptions(config.IfAbsent(), config.IfVersion(7)); err == nil {
		t.Fatal("combined write preconditions were accepted")
	}
}
