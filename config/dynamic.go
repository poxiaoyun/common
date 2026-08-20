package config

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	commonerrors "xiaoshiai.cn/common/errors"
)

// Object is a JSON object used as a Configuration value.
type Object map[string]any

// UnmarshalJSON decodes an object with json.Number values and rejects other roots.
func (object *Object) UnmarshalJSON(data []byte) error {
	decoded := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("configuration value must be a JSON object: %w", err)
	}
	if decoded == nil {
		return fmt.Errorf("configuration value must be a JSON object")
	}
	*object = decoded
	return nil
}

// EncodeObject converts value to an independent JSON object.
func EncodeObject(value any) (Object, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("encode configuration value: %v", err))
	}
	object := Object{}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&object); err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("configuration value must be a JSON object: %v", err))
	}
	if object == nil {
		return nil, commonerrors.NewBadRequest("configuration value must be a JSON object")
	}
	return object, nil
}

// Decode replaces target with the typed representation of Object.
func (object Object) Decode(target any) error {
	if target == nil {
		return commonerrors.NewBadRequest("configuration target must be a non-nil pointer")
	}
	targetValue := reflect.ValueOf(target)
	if targetValue.Kind() != reflect.Pointer || targetValue.IsNil() {
		return commonerrors.NewBadRequest("configuration target must be a non-nil pointer")
	}
	data, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("encode configuration object: %w", err)
	}
	decoded := reflect.New(targetValue.Elem().Type())
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(decoded.Interface()); err != nil {
		return fmt.Errorf("decode configuration object: %w", err)
	}
	targetValue.Elem().Set(decoded.Elem())
	return nil
}

// Configuration is one named, versioned JSON object.
type Configuration struct {
	Name    string `json:"name"`
	Version int64  `json:"version"`
	Value   Object `json:"value"`
}

// Key identifies one persisted Configuration and its current version.
type Key struct {
	Name    string `json:"name"`
	Version int64  `json:"version"`
}

// WriteOption applies an optional write precondition.
type WriteOption interface {
	ApplyToWrite(*WriteOptions)
}

// WriteOptions contains the resolved precondition for one write.
type WriteOptions struct {
	ExpectedVersion *int64
}

// IfVersionOption requires a Configuration to have a specific version.
type IfVersionOption int64

// ApplyToWrite applies the version precondition.
func (option IfVersionOption) ApplyToWrite(options *WriteOptions) {
	version := int64(option)
	options.ExpectedVersion = &version
}

// IfVersion requires the effective Configuration to have version. Version 0
// matches a Configuration that has not been persisted.
func IfVersion(version int64) IfVersionOption {
	return IfVersionOption(version)
}

// ResolveWriteOptions applies and validates options for one write.
func ResolveWriteOptions(options ...WriteOption) (WriteOptions, error) {
	resolved := WriteOptions{}
	for _, option := range options {
		if option == nil {
			return WriteOptions{}, commonerrors.NewBadRequest("configuration write options cannot be nil")
		}
		option.ApplyToWrite(&resolved)
	}
	if resolved.ExpectedVersion != nil && *resolved.ExpectedVersion < 0 {
		return WriteOptions{}, commonerrors.NewBadRequest("configuration version cannot be negative")
	}
	return resolved, nil
}

// PatchType identifies one supported JSON patch format.
type PatchType string

const (
	// MergePatch applies RFC 7396 JSON Merge Patch semantics.
	MergePatch PatchType = "application/merge-patch+json"
	// JSONPatch applies RFC 6902 JSON Patch semantics.
	JSONPatch PatchType = "application/json-patch+json"
)

// Patch is a patch against a Configuration value root.
type Patch struct {
	Type PatchType
	Data json.RawMessage
}

// Event contains one Configuration snapshot or a terminal Watch error.
type Event struct {
	Configuration Configuration
	Error         error
}

// Watcher provides an ordered snapshot stream for one Configuration.
type Watcher interface {
	// Events returns the ordered snapshot stream and closes after Stop or a terminal error.
	// The first successful event is always the current effective Configuration.
	Events() <-chan Event
	// Stop idempotently stops the Watcher and closes its event stream.
	Stop()
}

// DynamicConfig stores typed configuration objects addressed by namespace and name.
type DynamicConfig interface {
	// Set serializes object and replaces namespace/name according to options.
	Set(ctx context.Context, namespace, name string, object any, options ...WriteOption) (Configuration, error)
	// Get decodes the effective namespace/name value into object. Missing values
	// have Version 0 and an empty Value.
	Get(ctx context.Context, namespace, name string, object any) (Configuration, error)
	// Patch changes the effective namespace/name value and optionally decodes the result into object.
	Patch(ctx context.Context, namespace, name string, patch Patch, object any, options ...WriteOption) (Configuration, error)
	// ListKeys returns all persisted keys in namespace ordered by name.
	ListKeys(ctx context.Context, namespace string) ([]Key, error)

	// Watch observes Configuration snapshots and always starts with the current effective value.
	Watch(ctx context.Context, namespace, name string) (Watcher, error)
}

// OnChange watches one Configuration and decodes every snapshot into a fresh T.
func OnChange[T any](ctx context.Context, client DynamicConfig, namespace, name string, callback func(context.Context, T, int64) error) error {
	watcher, err := client.Watch(ctx, namespace, name)
	if err != nil {
		return err
	}
	defer watcher.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, open := <-watcher.Events():
			if !open {
				return nil
			}
			if event.Error != nil {
				return event.Error
			}
			object := *new(T)
			if err := event.Configuration.Value.Decode(&object); err != nil {
				return err
			}
			if err := callback(ctx, object, event.Configuration.Version); err != nil {
				return err
			}
		}
	}
}
