package config

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	commonerrors "xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/store"
)

// Configuration is one named, versioned JSON document.
type Configuration struct {
	store.ObjectMeta `json:",inline"`
	Value            json.RawMessage `json:"value"`
}

// WriteOption applies an optional write precondition.
type WriteOption interface {
	ApplyToWrite(*WriteOptions)
}

// WriteOptions contains the resolved preconditions for one write.
type WriteOptions struct {
	IfAbsent        bool
	ExpectedVersion *int64
}

// IfAbsentOption requires a Set target not to exist.
type IfAbsentOption struct{}

// ApplyToWrite applies the create-only precondition.
func (IfAbsentOption) ApplyToWrite(options *WriteOptions) {
	options.IfAbsent = true
}

// IfAbsent requires the Configuration not to exist. It is valid only for Set.
func IfAbsent() IfAbsentOption {
	return IfAbsentOption{}
}

// IfVersionOption requires a Configuration to have a specific version.
type IfVersionOption int64

// ApplyToWrite applies the version precondition.
func (option IfVersionOption) ApplyToWrite(options *WriteOptions) {
	version := int64(option)
	options.ExpectedVersion = &version
}

// IfVersion requires the Configuration to have version.
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
	if resolved.ExpectedVersion != nil && (*resolved.ExpectedVersion <= 0 || resolved.IfAbsent) {
		return WriteOptions{}, commonerrors.NewBadRequest("configuration write preconditions cannot be combined and versions must be positive")
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

// EventType identifies one high-level Configuration state transition.
type EventType string

const (
	// EventInitial is always the first Watch event. A nil Configuration means missing.
	EventInitial EventType = "initial"
	// EventChange represents a create or update after the initial state.
	EventChange EventType = "change"
	// EventDelete represents deletion after the initial state.
	EventDelete EventType = "delete"
)

// Event describes one Configuration Watch event.
type Event struct {
	Type          EventType
	Configuration *Configuration
	Error         error
}

// Watcher provides an ordered stream for one Configuration.
type Watcher interface {
	// Events returns the ordered event stream and closes after Stop or a terminal error.
	Events() <-chan Event
	// Stop idempotently stops the Watcher and closes its event stream.
	Stop()
}

// Change is the typed event delivered by OnChange.
type Change[T any] struct {
	Type    EventType
	Object  *T
	Version int64
}

// DynamicConfig stores typed configuration objects addressed by namespace and name.
type DynamicConfig interface {
	// Set serializes object and replaces namespace/name according to options.
	Set(ctx context.Context, namespace, name string, object any, options ...WriteOption) (*Configuration, error)
	// Get decodes namespace/name into object. A missing Configuration returns (nil, nil).
	Get(ctx context.Context, namespace, name string, object any) (*Configuration, error)
	// Patch changes namespace/name and optionally decodes the persisted result into object.
	Patch(ctx context.Context, namespace, name string, patch Patch, object any, options ...WriteOption) (*Configuration, error)
	// Watch observes one Configuration and always starts with exactly one Initial event.
	Watch(ctx context.Context, namespace, name string) (Watcher, error)
}

// OnChange watches one Configuration and decodes every state into a fresh T.
func OnChange[T any](ctx context.Context, client DynamicConfig, namespace, name string, callback func(context.Context, Change[T]) error) error {
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
			change := Change[T]{Type: event.Type}
			if event.Configuration != nil {
				object := new(T)
				decoder := json.NewDecoder(bytes.NewReader(event.Configuration.Value))
				decoder.UseNumber()
				if err := decoder.Decode(object); err != nil {
					return fmt.Errorf("decode configuration object: %w", err)
				}
				change.Object = object
				change.Version = event.Configuration.ResourceVersion
			}
			if err := callback(ctx, change); err != nil {
				return err
			}
		}
	}
}
