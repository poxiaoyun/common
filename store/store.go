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
		Sort            string
		ResourceVersion int64
		LabelSelector   labels.Selector
		FieldSelector   fields.Selector
		//  IncludeSubScopes is a flag to include resources in subscopes of current scope.
		IncludeSubScopes bool
	}
	ListOption func(*ListOptions)

	CountOptions struct {
		LabelSelector    labels.Selector
		FieldSelector    fields.Selector
		IncludeSubScopes bool
	}
	CountOption func(*CountOptions)

	CreateOptions struct {
		TTL time.Duration
	}
	CreateOption func(*CreateOptions)

	DeleteOptions struct {
		PropagationPolicy *DeletionPropagation
	}
	DeleteOption func(*DeleteOptions)

	UpdateOptions struct {
		TTL time.Duration
	}
	UpdateOption func(*UpdateOptions)

	PatchOptions struct{}
	PatchOption  func(*PatchOptions)

	WatchOptions struct {
		LabelSelector    labels.Selector
		FieldSelector    fields.Selector
		ResourceVersion  int64
		IncludeSubScopes bool
	}
	WatchOption func(*WatchOptions)
)

func WithCountFieldSelector(selector fields.Selector) CountOption {
	return func(o *CountOptions) {
		o.FieldSelector = selector
	}
}

func WithFieldSelector(sel fields.Selector) ListOption {
	return func(o *ListOptions) {
		o.FieldSelector = sel
	}
}

func WithFieldSelectorFromSet(kvs map[string]string) ListOption {
	return func(o *ListOptions) {
		o.FieldSelector = fields.SelectorFromSet(fields.Set(kvs))
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
		o.LabelSelector = labels.SelectorFromSet(labels.Set(kvs))
	}
}

func WithLabelSelector(selector labels.Selector) ListOption {
	return func(o *ListOptions) {
		o.LabelSelector = selector
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

type Requirements []Requirement

func RequirementEqual(key string, value string) Requirement {
	return Requirement{
		Key:      key,
		Operator: OperatorEquals,
		Values:   []string{value},
	}
}

func NewRequirement(key string, operator Operator, values ...string) Requirement {
	return Requirement{
		Key:      key,
		Operator: operator,
		Values:   values,
	}
}

type Requirement struct {
	Key      string
	Operator Operator
	Values   []string
}

type Operator string

const (
	OperatorEquals      Operator = Operator("=")
	OperatorNotEquals   Operator = Operator("!=")
	OperatorIn          Operator = Operator("in")
	OperatorNotIn       Operator = Operator("notin")
	OperatorExists      Operator = Operator("exists")
	OperatorNotExists   Operator = Operator("!")
	OperatorGreaterThan Operator = Operator("gt")
	OperatorLessThan    Operator = Operator("lt")
	OperatorContains    Operator = Operator("contains")
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
