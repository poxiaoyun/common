package store

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	FinalizerOrphanDependents = "orphan"
	FinalizerDeleteDependents = "foregroundDeletion"
)

type (
	GetOptions struct {
		ResourceVersion int64
		// FieldRequirements is a list of conditions that must be true for the get to occur.
		// It may not supported by all databases.
		FieldRequirements Requirements
		LabelRequirements Requirements
	}
	GetOption func(*GetOptions)

	ListOptions struct {
		Page   int
		Size   int
		Search string
		// Sort is the sort order of the list.  The format is a comma separated list of fields, optionally
		// prefixed by "+" or "-".  The default is "+metadata.name", which sorts by the object's name.
		// For example, "-metadata.name,metadata.creationTimestamp" sorts first by descending name, and then by
		// ascending creation timestamp.
		// name is alias for metadata.name
		// time is alias for metadata.creationTimestamp
		Sort              string
		ResourceVersion   int64
		LabelRequirements Requirements
		FieldRequirements Requirements
		//  IncludeSubScopes is a flag to include resources in subscopes of current scope.
		IncludeSubScopes bool
		Continue         string
	}
	ListOption func(*ListOptions)

	CountOptions struct {
		LabelRequirements Requirements
		FieldRequirements Requirements
		IncludeSubScopes  bool
	}
	CountOption func(*CountOptions)

	CreateOptions struct {
		TTL time.Duration
		// AutoIncrementOnName is a flag to enable auto increment id for object
		// it'll set the object's name to the auto increment id if empty
		AutoIncrementOnName bool
	}
	CreateOption func(*CreateOptions)

	DeleteOptions struct {
		PropagationPolicy *DeletionPropagation
	}
	DeleteOption func(*DeleteOptions)

	UpdateOptions struct {
		TTL time.Duration
		// FieldRequirements is a list of conditions that must be true for the update to occur.
		// it apply to fields.
		FieldRequirements Requirements
		LabelRequirements Requirements
	}
	UpdateOption func(*UpdateOptions)

	PatchOptions struct {
		FieldRequirements Requirements
		LabelRequirements Requirements
	}
	PatchOption func(*PatchOptions)

	WatchOptions struct {
		LabelRequirements Requirements
		FieldRequirements Requirements
		ResourceVersion   int64
		IncludeSubScopes  bool
		SendInitialEvents bool
	}
	WatchOption func(*WatchOptions)
)

func WithSendInitialEvents() WatchOption {
	return func(o *WatchOptions) {
		o.SendInitialEvents = true
	}
}

func WithWatchSubscopes() WatchOption {
	return func(o *WatchOptions) {
		o.IncludeSubScopes = true
	}
}

func WithWatchFieldRequirements(reqs ...Requirement) WatchOption {
	return func(o *WatchOptions) {
		o.FieldRequirements = append(o.FieldRequirements, reqs...)
	}
}

func WithCountFieldRequirementsFromSelector(selector fields.Selector) CountOption {
	return func(o *CountOptions) {
		o.FieldRequirements = append(o.FieldRequirements, FieldsSelectorToReqirements(selector)...)
	}
}

func WithGetFieldRequirements(reqs ...Requirement) GetOption {
	return func(o *GetOptions) {
		o.FieldRequirements = append(o.FieldRequirements, reqs...)
	}
}

func WithGetLabelRequirements(reqs ...Requirement) GetOption {
	return func(o *GetOptions) {
		o.LabelRequirements = append(o.LabelRequirements, reqs...)
	}
}

func WithUpdateFieldRequirements(reqs ...Requirement) UpdateOption {
	return func(o *UpdateOptions) {
		o.FieldRequirements = append(o.FieldRequirements, reqs...)
	}
}

func WithCountFieldRequirementsFromSet(kvs map[string]string) CountOption {
	return func(o *CountOptions) {
		o.FieldRequirements = append(o.FieldRequirements, RequirementsFromMap(kvs)...)
	}
}

func WithFieldRequirementsFromSelector(selector fields.Selector) ListOption {
	return func(o *ListOptions) {
		o.FieldRequirements = append(o.FieldRequirements, FieldsSelectorToReqirements(selector)...)
	}
}

func WithContinue(token string) ListOption {
	return func(o *ListOptions) {
		o.Continue = token
	}
}

func WithFieldRequirementsFromSet(kvs map[string]string) ListOption {
	return func(o *ListOptions) {
		o.FieldRequirements = append(o.FieldRequirements, RequirementsFromMap(kvs)...)
	}
}

func WithFieldRequirements(reqs ...Requirement) ListOption {
	return func(o *ListOptions) {
		o.FieldRequirements = append(o.FieldRequirements, reqs...)
	}
}

func WithPageSize(page, size int) ListOption {
	return func(o *ListOptions) {
		o.Page = page
		o.Size = size
	}
}

func WithSort(sort string) ListOption {
	return func(o *ListOptions) {
		o.Sort = sort
	}
}

func WithSearch(search string) ListOption {
	return func(o *ListOptions) {
		o.Search = search
	}
}

func WithMatchLabels(kvs map[string]string) ListOption {
	return func(o *ListOptions) {
		o.LabelRequirements = append(o.LabelRequirements, RequirementsFromMap(kvs)...)
	}
}

func WithLabelRequirementsFromSelector(selector labels.Selector) ListOption {
	return func(o *ListOptions) {
		o.LabelRequirements = append(o.LabelRequirements, LabelsSelectorToReqirements(selector)...)
	}
}

func WithLabelRequirements(reqs ...Requirement) ListOption {
	return func(o *ListOptions) {
		o.LabelRequirements = append(o.LabelRequirements, reqs...)
	}
}

func WithSubScopes() ListOption {
	return func(o *ListOptions) {
		o.IncludeSubScopes = true
	}
}

// DeletionPropagation decides if a deletion will propagate to the dependents of
// the object, and how the garbage collector will handle the propagation.
type DeletionPropagation string

const (
	DeletePropagationBackground DeletionPropagation = "Background"
	DeletePropagationForeground DeletionPropagation = "Foreground"
	DeletePropagationOrphan     DeletionPropagation = "Orphan"
)

type PatchType string

const (
	PatchTypeJSONPatch  PatchType = "application/json-patch+json"
	PatchTypeMergePatch PatchType = "application/merge-patch+json"
)

type Patch interface {
	Type() PatchType
	Data(obj Object) ([]byte, error)
}

type Watcher interface {
	Stop()
	Events() <-chan WatchEvent
}
type WatchEventType string

const (
	WatchEventCreate   WatchEventType = "create"
	WatchEventUpdate   WatchEventType = "update"
	WatchEventDelete   WatchEventType = "delete"
	WatchEventBookmark WatchEventType = "bookmark"
)

type WatchEvent struct {
	Type   WatchEventType
	Error  error
	Object Object
}

func WithDeletePropagation(policy DeletionPropagation) DeleteOption {
	return func(o *DeleteOptions) {
		o.PropagationPolicy = &policy
	}
}

type Store interface {
	Get(ctx context.Context, name string, obj Object, opts ...GetOption) error
	List(ctx context.Context, list ObjectList, opts ...ListOption) error
	Count(ctx context.Context, obj Object, opts ...CountOption) (int, error)
	Create(ctx context.Context, obj Object, opts ...CreateOption) error
	Delete(ctx context.Context, obj Object, opts ...DeleteOption) error
	Update(ctx context.Context, obj Object, opts ...UpdateOption) error
	Patch(ctx context.Context, obj Object, patch Patch, opts ...PatchOption) error
	Watch(ctx context.Context, obj ObjectList, opts ...WatchOption) (Watcher, error)
	Status() StatusStorage
	Scope(scope ...Scope) Store
}
type StatusStorage interface {
	Update(ctx context.Context, obj Object, opts ...UpdateOption) error
	Patch(ctx context.Context, obj Object, patch Patch, opts ...PatchOption) error
}

type TransactionOptions struct {
	Timeout    time.Duration
	MaxRetries int
}

type TransactionOption func(*TransactionOptions)

func WithTransactionTimeout(timeout time.Duration) TransactionOption {
	return func(o *TransactionOptions) {
		o.Timeout = timeout
	}
}

func WithTransactionMaxRetries(retries int) TransactionOption {
	return func(o *TransactionOptions) {
		o.MaxRetries = retries
	}
}

type TransactionStore interface {
	Store
	Transcation(ctx context.Context, fn func(ctx context.Context, store Store) error, opts ...TransactionOption) error
}

// AutoIncrementID is a type for auto increment id
// impletions should use this type for auto increment id
type AutoIncrementID uint64
