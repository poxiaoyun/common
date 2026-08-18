package store

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/utils/ptr"
	"xiaoshiai.cn/common/meta"
)

const (
	FinalizerOrphanDependents = "orphan"
	FinalizerDeleteDependents = "foregroundDeletion"
)

type (
	GetOptions struct {
		// ResourceVersion set to 0 to get from cache
		// ResourceVersion set to a specific value to get the object not older than that version
		// ResourceVersion set to nil to get from backend
		ResourceVersion *int64
		// FieldRequirements is a list of conditions that must be true for the get to occur.
		// It may not supported by all databases.
		FieldRequirements Requirements
		LabelRequirements Requirements
		// Fields is a list of fields to return.  If empty, all fields are returned.
		Fields []string
	}
	GetOption func(*GetOptions)

	ListOptions struct {
		// Page selects the pagination model. Values greater than zero use
		// page/size pagination; zero uses backend continuation pagination.
		Page int
		// Size limits the returned items. Zero returns all matching items.
		Size   int
		Search string
		// SearchFields overrides the default search fields, id and name.
		SearchFields []string
		// Sort is the sort order of the list. The format is a comma-separated list of fields, optionally
		// suffixed by "+" or "-". The default is "metadata.name+", which sorts by the object's name.
		// For example, "metadata.name-,metadata.creationTimestamp+" sorts first by descending name, and then by
		// ascending creation timestamp.
		// name is alias for metadata.name
		// time is alias for metadata.creationTimestamp
		Sort string
		// ResourceVersion set to 0 to get from cache
		// ResourceVersion set to a specific value to get the object not older than that version
		// ResourceVersion set to nil to get from backend
		ResourceVersion   *int64
		LabelRequirements Requirements
		FieldRequirements Requirements
		//  IncludeSubScopes is a flag to include resources in subscopes of current scope.
		IncludeSubScopes bool
		// Continue is the opaque token returned by a previous list request.
		Continue string
		// Fields is a list of fields to return.  If empty, all fields are returned.
		Fields []string
	}
	ListOption func(*ListOptions)

	CountOptions struct {
		LabelRequirements Requirements
		FieldRequirements Requirements
		IncludeSubScopes  bool
	}
	CountOption func(*CountOptions)

	CreateOptions struct {
		TTL    time.Duration
		DryRun bool
	}
	CreateOption func(*CreateOptions)

	DeleteOptions struct {
		LabelRequirements Requirements
		// FieldRequirements is not supported by all databases on deletion.
		FieldRequirements Requirements
		Preconditions     *Preconditions
		PropagationPolicy *DeletionPropagation
		DryRun            bool
	}
	DeleteOption func(*DeleteOptions)
	// Preconditions restrict deletion to one caller-observed object identity.
	Preconditions struct {
		// UID prevents deleting a replacement object that reused the same ID.
		UID *string
		// ResourceVersion restricts deletion to the observed object version.
		ResourceVersion *int64
	}

	DeleteBatchOptions struct {
		LabelRequirements Requirements
		FieldRequirements Requirements
		DryRun            bool
	}
	DeleteBatchOption func(*DeleteBatchOptions)

	UpdateOptions struct {
		TTL time.Duration
		// FieldRequirements is a list of conditions that must be true for the update to occur.
		// it apply to fields.
		FieldRequirements Requirements
		LabelRequirements Requirements
		DryRun            bool
	}
	UpdateOption func(*UpdateOptions)

	PatchOptions struct {
		FieldRequirements Requirements
		LabelRequirements Requirements
		DryRun            bool
	}
	PatchOption func(*PatchOptions)

	PatchBatchOptions struct {
		// FieldRequirements is a list of conditions that must be true for the update to occur.
		// it apply to fields.
		FieldRequirements Requirements
		LabelRequirements Requirements
		DryRun            bool
	}
	PatchBatchOption func(*PatchBatchOptions)

	WatchOptions struct {
		ID                string
		LabelRequirements Requirements
		FieldRequirements Requirements
		ResourceVersion   *int64
		IncludeSubScopes  bool
		SendInitialEvents bool
	}
	WatchOption func(*WatchOptions)
)

// WithSendInitialEvents requests an initial Watch. All events before the first
// Bookmark build an authoritative snapshot; later events are live changes.
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

func WithWatchID(id string) WatchOption {
	return func(o *WatchOptions) {
		o.ID = id
	}
}

// WithWatchResourceVersion requests events after the supplied global watch
// version. Stores return ResourceExpired when that position cannot be served;
// callers can then rebuild from initial events.
func WithWatchResourceVersion(resourceVersion int64) WatchOption {
	return func(o *WatchOptions) {
		o.ResourceVersion = ptr.To(resourceVersion)
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

func WithCountFieldRequirements(reqs ...Requirement) CountOption {
	return func(o *CountOptions) {
		o.FieldRequirements = append(o.FieldRequirements, reqs...)
	}
}

func WithCountLabelRequirements(reqs ...Requirement) CountOption {
	return func(o *CountOptions) {
		o.LabelRequirements = append(o.LabelRequirements, reqs...)
	}
}

func WithCountSubScopes() CountOption {
	return func(o *CountOptions) {
		o.IncludeSubScopes = true
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

func WithGetFields(fields ...string) GetOption {
	return func(o *GetOptions) {
		o.Fields = append(o.Fields, fields...)
	}
}

func WithGetResourceVersion(rv int64) GetOption {
	return func(o *GetOptions) {
		o.ResourceVersion = ptr.To(rv)
	}
}

func WithUpdateFieldRequirements(reqs ...Requirement) UpdateOption {
	return func(o *UpdateOptions) {
		o.FieldRequirements = append(o.FieldRequirements, reqs...)
	}
}

func WithUpdateLabelRequirements(reqs ...Requirement) UpdateOption {
	return func(o *UpdateOptions) {
		o.LabelRequirements = append(o.LabelRequirements, reqs...)
	}
}

func WithUpdateDryRun(dryRun bool) UpdateOption {
	return func(o *UpdateOptions) {
		o.DryRun = dryRun
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

func WithFields(fields ...string) ListOption {
	return func(o *ListOptions) {
		o.Fields = append(o.Fields, fields...)
	}
}

// WithResourceVersion set 0 to read from latest cache
// WithResourceVersion set to -1 to read from backend
func WithResourceVersion(rv int64) ListOption {
	return func(o *ListOptions) {
		if rv < 0 {
			o.ResourceVersion = nil
		} else {
			o.ResourceVersion = ptr.To(rv)
		}
	}
}

func WithPatchFieldRequirements(reqs ...Requirement) PatchOption {
	return func(o *PatchOptions) {
		o.FieldRequirements = append(o.FieldRequirements, reqs...)
	}
}

func WithPatchLabelRequirements(reqs ...Requirement) PatchOption {
	return func(o *PatchOptions) {
		o.LabelRequirements = append(o.LabelRequirements, reqs...)
	}
}

func WithPatchBatchFieldRequirements(reqs ...Requirement) PatchBatchOption {
	return func(o *PatchBatchOptions) {
		o.FieldRequirements = append(o.FieldRequirements, reqs...)
	}
}

func WithPatchBatchLabelRequirements(reqs ...Requirement) PatchBatchOption {
	return func(o *PatchBatchOptions) {
		o.LabelRequirements = append(o.LabelRequirements, reqs...)
	}
}

func WithDeleteBatchFieldRequirements(reqs ...Requirement) DeleteBatchOption {
	return func(o *DeleteBatchOptions) {
		o.FieldRequirements = append(o.FieldRequirements, reqs...)
	}
}

func WithDeleteBatchLabelRequirements(reqs ...Requirement) DeleteBatchOption {
	return func(o *DeleteBatchOptions) {
		o.LabelRequirements = append(o.LabelRequirements, reqs...)
	}
}

func WithDeleteFieldRequirements(reqs ...Requirement) DeleteOption {
	return func(o *DeleteOptions) {
		o.FieldRequirements = append(o.FieldRequirements, reqs...)
	}
}

func WithDeleteLabelRequirements(reqs ...Requirement) DeleteOption {
	return func(o *DeleteOptions) {
		o.LabelRequirements = append(o.LabelRequirements, reqs...)
	}
}

// WithDeletePreconditions applies non-zero caller-facing identity conditions.
// Empty UID and zero ResourceVersion mean the caller omitted that condition.
func WithDeletePreconditions(preconditions meta.Preconditions) DeleteOption {
	return func(o *DeleteOptions) {
		if preconditions.UID != "" {
			if o.Preconditions == nil {
				o.Preconditions = &Preconditions{}
			}
			o.Preconditions.UID = ptr.To(preconditions.UID)
		}
		if preconditions.ResourceVersion != 0 {
			if o.Preconditions == nil {
				o.Preconditions = &Preconditions{}
			}
			o.Preconditions.ResourceVersion = ptr.To(preconditions.ResourceVersion)
		}
	}
}

// WithDeleteUID restricts deletion to an object with uid.
func WithDeleteUID(uid string) DeleteOption {
	return func(o *DeleteOptions) {
		if o.Preconditions == nil {
			o.Preconditions = &Preconditions{}
		}
		o.Preconditions.UID = ptr.To(uid)
	}
}

// WithDeleteResourceVersion restricts deletion to an observed object version.
func WithDeleteResourceVersion(resourceVersion int64) DeleteOption {
	return func(o *DeleteOptions) {
		if o.Preconditions == nil {
			o.Preconditions = &Preconditions{}
		}
		o.Preconditions.ResourceVersion = ptr.To(resourceVersion)
	}
}

func WithTTL(ttl time.Duration) CreateOption {
	return func(o *CreateOptions) {
		o.TTL = ttl
	}
}

func WithDryRun(dryRun bool) CreateOption {
	return func(o *CreateOptions) {
		o.DryRun = dryRun
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

func WithSearchFields(fields ...string) ListOption {
	return func(o *ListOptions) {
		o.SearchFields = append(o.SearchFields, fields...)
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

type PatchBatch interface {
	Type() PatchType
	Data() []byte
}

type Watcher interface {
	// Stop stops the watch. It is safe to call more than once. Events eventually
	// closes after Stop or cancellation of the context passed to Store.Watch.
	Stop()
	// Events returns the ordered event stream. A runtime failure is delivered as
	// one terminal error event before the channel closes.
	Events() <-chan WatchEvent
}
type WatchEventType string

const (
	WatchEventCreate   WatchEventType = "create"
	WatchEventUpdate   WatchEventType = "update"
	WatchEventDelete   WatchEventType = "delete"
	WatchEventBookmark WatchEventType = "bookmark"
)

// WatchEvent describes one ordered change in a Watch stream. ResourceVersion
// is a global stream checkpoint when positive; Object.ResourceVersion remains
// the version of that individual object.
type WatchEvent struct {
	Type            WatchEventType
	Object          Object
	ResourceVersion int64
	Error           error
}

func WithDeletePropagation(policy DeletionPropagation) DeleteOption {
	return func(o *DeleteOptions) {
		o.PropagationPolicy = &policy
	}
}

// Store persists scoped resources and owns their lifecycle metadata.
type Store interface {
	// Schema returns an independent snapshot of the resource schema. Mutating
	// the returned schema does not affect the Store or Stores derived by Scope.
	Schema() *Schema
	// Capabilities reports the optional behavior implemented by this store.
	Capabilities() Capabilities
	// Get loads the object identified by id in the exact current scope into obj.
	// A selector mismatch is reported as NotFound.
	Get(ctx context.Context, id string, obj Object, opts ...GetOption) error
	// List loads objects from the current scope into list. Order is unspecified
	// unless a sort is requested; subscopes are excluded by default.
	List(ctx context.Context, list ObjectList, opts ...ListOption) error
	// Count returns the number of objects matching the selectors in the exact
	// current scope, or its subscopes when explicitly requested.
	Count(ctx context.Context, obj Object, opts ...CountOption) (int, error)
	// Create persists obj in the current scope. It generates missing identity
	// and server-owned metadata and returns the persisted metadata through obj.
	Create(ctx context.Context, obj Object, opts ...CreateOption) error
	// Delete requests deletion of obj in the current scope. Background deletion
	// is the default; finalizers can keep the object in a terminating state.
	Delete(ctx context.Context, obj Object, opts ...DeleteOption) error
	// DeleteBatch deletes objects of the list's resource that match the supplied
	// selectors in the current scope.
	DeleteBatch(ctx context.Context, obj ObjectList, opts ...DeleteBatchOption) error
	// Update replaces obj while preserving server-owned metadata and status.
	// ResourceVersion zero is unconditional; a non-zero value is a CAS condition.
	Update(ctx context.Context, obj Object, opts ...UpdateOption) error
	// Patch atomically applies patch to the latest stored object. The object's
	// ResourceVersion is not a precondition; a non-zero resourceVersion in a
	// merge patch or a JSON Patch test operation makes the patch conditional.
	Patch(ctx context.Context, obj Object, patch Patch, opts ...PatchOption) error
	// PatchBatch applies patch to objects of the list's resource that match the
	// supplied selectors in the current scope.
	PatchBatch(ctx context.Context, obj ObjectList, patch PatchBatch, opts ...PatchBatchOption) error
	// Watch observes ordered mutations of the list's resource without silently
	// dropping events. Delete events contain the complete previous object and
	// selector transitions are reported as Create, Update, or Delete according
	// to membership before and after the mutation. WithSendInitialEvents starts
	// an initial Watch: callers apply every event before the first Bookmark to
	// staging state, then atomically publish it; later events are live changes.
	// A positive WatchEvent ResourceVersion is a global checkpoint accepted by
	// WithWatchResourceVersion. Stores that cannot serve a requested position
	// return ResourceExpired so callers can start a new initial Watch.
	Watch(ctx context.Context, obj ObjectList, opts ...WatchOption) (Watcher, error)
	// Status returns the status-subresource writer. Its mutations can only change
	// the top-level status field and ResourceVersion.
	Status() StatusStorage
	// Scope returns a derived Store with scope appended to the current ordered
	// scope; it does not mutate the receiver.
	Scope(scope ...Scope) Store
}

// Capabilities describes optional Store behavior for discovery. Store methods
// validate options from their own backend constraints, not from this value.
// CRUD, Count, exact scopes, server-managed metadata, status isolation,
// finalizers, and ID sorting are part of the base contract and are omitted.
type Capabilities struct {
	LabelSelector    bool
	FieldSelector    bool
	Search           bool
	Sort             bool
	Page             bool
	Continue         bool
	ContinueWithSort bool
	Projection       bool
	SubScopes        bool
	OptimisticLock   bool
	// Watch reports support for the complete snapshot-to-live Watch contract.
	Watch            bool
	DeleteBatch      bool
	PatchBatch       bool
	Transaction      bool
	TTL              bool
	DryRun           bool
	SecondaryIndexes bool
	UniqueIndexes    bool
}

// Pinger reports whether a backend can currently serve requests. Applications
// may use it as one input to readiness without making it part of Store's data
// operation contract.
type Pinger interface {
	Ping(ctx context.Context) error
}

type PingableStore interface {
	Store
	Pinger
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
	Transaction(ctx context.Context, fn func(ctx context.Context, store Store) error, opts ...TransactionOption) error
}
