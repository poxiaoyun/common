package etcdcache

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.etcd.io/etcd/client/pkg/v3/transport"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/storage"
	cacherstorage "k8s.io/apiserver/pkg/storage/cacher"
	storeerr "k8s.io/apiserver/pkg/storage/errors"
	"k8s.io/apiserver/pkg/storage/etcd3"
	"k8s.io/apiserver/pkg/storage/value/encrypt/identity"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/ptr"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/store"
)

type Options struct {
	Servers       []string `json:"servers,omitempty"`
	Username      string   `json:"username,omitempty"`
	Password      string   `json:"password,omitempty"`
	KeyFile       string   `json:"keyFile,omitempty"`
	CertFile      string   `json:"certFile,omitempty"`
	TrustedCAFile string   `json:"trustedCAFile,omitempty"`
	KeyPrefix     string   `json:"keyPrefix,omitempty"`
}

func NewDefaultOptions() *Options {
	return &Options{
		Servers:   []string{"http://127.0.0.1:2379"},
		KeyPrefix: "/core",
	}
}

const (
	// The short keepalive timeout and interval have been chosen to aggressively
	// detect a failed etcd server without introducing much overhead.
	keepaliveTime    = 30 * time.Second
	keepaliveTimeout = 10 * time.Second

	// dialTimeout is the timeout for failing to establish a connection.
	// It is set to 20 seconds as times shorter than that will cause TLS connections to fail
	// on heavily loaded arm64 CPUs (issue #64649)
	dialTimeout = 20 * time.Second

	dbMetricsMonitorJitter = 0.5
)

func NewETCD3Client(c *Options) (*clientv3.Client, error) {
	tlsInfo := transport.TLSInfo{
		CertFile:      c.CertFile,
		KeyFile:       c.KeyFile,
		TrustedCAFile: c.TrustedCAFile,
	}
	tlsConfig, err := tlsInfo.ClientConfig()
	if err != nil {
		return nil, err
	}
	// NOTE: Client relies on nil tlsConfig
	// for non-secure connections, update the implicit variable
	if len(c.CertFile) == 0 && len(c.KeyFile) == 0 && len(c.TrustedCAFile) == 0 {
		tlsConfig = nil
	}
	dialOptions := []grpc.DialOption{}
	cfg := clientv3.Config{
		DialTimeout:          dialTimeout,
		DialKeepAliveTime:    keepaliveTime,
		DialKeepAliveTimeout: keepaliveTimeout,
		DialOptions:          dialOptions,
		Endpoints:            c.Servers,
		TLS:                  tlsConfig,
		Username:             c.Username,
		Password:             c.Password,
	}
	return clientv3.New(cfg)
}

func NewEtcdCacher(options *Options, resFields ResourceFieldsMap) (*generic, error) {
	cli, err := NewETCD3Client(options)
	if err != nil {
		return nil, err
	}
	return NewEtcdCacherFromClient(cli, options.KeyPrefix, resFields)
}

func NewEtcdCacherFromClient(cli *clientv3.Client, storagePrefix string, resFields ResourceFieldsMap) (*generic, error) {
	if resFields == nil {
		resFields = make(map[string][]string)
	}
	core := &core{
		storagePrefix:  storagePrefix,
		cli:            cli,
		resources:      make(map[string]*db),
		resourceFields: resFields,
	}
	return &generic{core: core}, nil
}

var _ store.Store = &generic{}

type generic struct {
	core   *core
	scopes []store.Scope
}

// Count implements store.Store.
func (c *generic) Count(ctx context.Context, obj store.Object, opts ...store.CountOption) (int, error) {
	options := store.CountOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	preficate, err := ConvertPredicate(options.LabelRequirements, options.FieldRequirements)
	if err != nil {
		return 0, err
	}
	count := 0
	if err := c.core.on(ctx, obj, func(ctx context.Context, db *db) error {
		key := getlistkey(c.scopes, db.resource.String())
		listopts := storage.ListOptions{Recursive: true, Predicate: preficate}
		list := &unstructured.UnstructuredList{}
		if err := db.storage.GetList(ctx, key, listopts, list); err != nil {
			return err
		}
		// filter
		filtered := list.Items
		if !options.IncludeSubScopes {
			filtered = FilterByScopes(filtered, c.scopes)
		}
		count = len(filtered)
		return nil
	}); err != nil {
		return 0, err
	}
	return count, nil
}

// Create implements store.Store.
func (c *generic) Create(ctx context.Context, obj store.Object, opts ...store.CreateOption) error {
	options := store.CreateOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	return c.core.on(ctx, obj, func(ctx context.Context, db *db) error {
		if obj.GetName() == "" {
			return errors.NewBadRequest(fmt.Sprintf("name is required for %s", db.resource))
		}
		obj.SetUID(uuid.New().String())
		obj.SetCreationTimestamp(store.Now())
		obj.SetScopes(c.scopes)
		obj.SetResource(db.resource.String())
		uns, err := ConvertToUnstructured(obj)
		if err != nil {
			return err
		}
		key := getObjectKey(c.scopes, db.resource.String(), obj.GetName())
		if err := db.storage.Create(ctx, key, uns, uns, uint64(options.TTL.Seconds())); err != nil {
			err = storeerr.InterpretCreateError(err, db.resource, obj.GetName())
			return err
		}
		ConvertFromUnstructured(uns, obj)
		return nil
	})
}

// Delete implements store.Store.
func (c *generic) Delete(ctx context.Context, obj store.Object, opts ...store.DeleteOption) error {
	options := store.DeleteOptions{
		PropagationPolicy: ptr.To(store.DeletePropagationForeground),
	}
	for _, opt := range opts {
		opt(&options)
	}
	preconditions := &storage.Preconditions{}
	if obj.GetUID() != "" {
		preconditions.UID = ptr.To(types.UID(obj.GetUID()))
	}
	updatefunc := func(ctx context.Context, current *store.Unstructured) (newObj store.Object, err error) {
		// update finalizers
		if options.PropagationPolicy != nil {
			gcFinalizers := []string{}
			switch *options.PropagationPolicy {
			case store.DeletePropagationForeground:
				gcFinalizers = append(gcFinalizers, store.FinalizerDeleteDependents)
			}
			nogcFinalizers := slices.DeleteFunc(current.GetFinalizers(), func(finalizer string) bool {
				return finalizer == store.FinalizerDeleteDependents
			})
			finalizers := append(nogcFinalizers, gcFinalizers...)
			current.SetFinalizers(finalizers)
			unstructured.SetNestedStringSlice(current.Object, finalizers, "finalizers")
		}
		if current.GetDeletionTimestamp() == nil {
			now := metav1.Now()
			current.SetDeletionTimestamp(ptr.To(now))
			unstructured.SetNestedField(current.Object, now.Format(time.RFC3339), "deletionTimestamp")
		}
		return current, nil
	}
	return c.update(ctx, obj, preconditions, updatefunc)
}

// Get implements store.Store.
func (c *generic) Get(ctx context.Context, name string, obj store.Object, opts ...store.GetOption) error {
	options := store.GetOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	return c.core.on(ctx, obj, func(ctx context.Context, db *db) error {
		key := getObjectKey(c.scopes, db.resource.String(), name)
		uns := &unstructured.Unstructured{}
		if err := db.storage.Get(ctx, key, storage.GetOptions{}, uns); err != nil {
			err = storeerr.InterpretGetError(err, db.resource, name)
			return err
		}
		return ConvertFromUnstructured(uns, obj)
	})
}

// List implements store.Store.
func (c *generic) List(ctx context.Context, list store.ObjectList, opts ...store.ListOption) error {
	options := store.ListOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	preficate, err := ConvertPredicate(options.LabelRequirements, options.FieldRequirements)
	if err != nil {
		return err
	}
	v, newItemFunc, err := store.NewItemFuncFromList(list)
	if err != nil {
		return err
	}
	return c.core.on(ctx, list, func(ctx context.Context, db *db) error {
		keyprefix := getlistkey(c.scopes, db.resource.String())
		listopts := storage.ListOptions{Recursive: true, Predicate: preficate}
		if options.ResourceVersion != 0 {
			listopts.ResourceVersion = strconv.FormatInt(options.ResourceVersion, 10)
		}

		unslist := &unstructured.UnstructuredList{}
		const MaxRetry = 3
		for retries := 0; ; retries++ {
			if err := db.storage.GetList(ctx, keyprefix, listopts, unslist); err != nil {
				// is retryable
				if retries < MaxRetry && apierrors.IsTooManyRequests(err) {
					if delay, ok := apierrors.SuggestsClientDelay(err); ok {
						time.Sleep(time.Duration(delay) * time.Second)
						continue
					}
				}
				err = storeerr.InterpretListError(err, db.resource)
				return err
			} else {
				break
			}
		}
		// filter
		filtered := unslist.Items
		// scopes
		if !options.IncludeSubScopes {
			filtered = FilterByScopes(filtered, c.scopes)
		}
		// sort
		SortUnstructuredList(filtered, options.Sort)
		// pagination
		total := len(filtered)
		filtered = PageUnstructuredList(filtered, options.Page, options.Size)

		// convert to result
		for _, uns := range filtered {
			obj := newItemFunc()
			if err := ConvertFromUnstructured(&uns, obj); err != nil {
				return err
			}
			v.Set(reflect.Append(v, reflect.ValueOf(obj).Elem()))
		}
		list.SetPage(options.Page)
		list.SetSize(options.Size)
		list.SetToal(total)
		rev, _ := strconv.ParseInt(unslist.GetResourceVersion(), 10, 64)
		list.SetResourceVersion(rev)
		list.SetScopes(c.scopes)
		list.SetResource(db.resource.String())
		return nil
	})
}

func ConvertPredicate(l store.Requirements, f store.Requirements) (storage.SelectionPredicate, error) {
	labelssel := labels.Everything()
	fieldsel := fields.Everything()
	if l != nil {
		newlabelssel, err := requirementsToLabelsSelector(l)
		if err != nil {
			return storage.SelectionPredicate{}, err
		}
		labelssel = newlabelssel
	}
	if f != nil {
		newfieldsel, err := requirementsToFieldsSelector(f)
		if err != nil {
			return storage.SelectionPredicate{}, err
		}
		fieldsel = newfieldsel
	}
	return storage.SelectionPredicate{Label: labelssel, Field: fieldsel}, nil
}

func requirementsToLabelsSelector(reqs store.Requirements) (labels.Selector, error) {
	selector := labels.Everything()
	for _, req := range reqs {
		labelreq, err := labels.NewRequirement(req.Key, req.Operator, req.Values)
		if err != nil {
			return nil, err
		}
		selector = selector.Add(*labelreq)
	}
	return selector, nil
}

func requirementsToFieldsSelector(reqs store.Requirements) (fields.Selector, error) {
	selectors := make([]fields.Selector, 0, len(reqs))
	for _, req := range reqs {
		switch req.Operator {
		case selection.Equals, selection.DoubleEquals:
			selectors = append(selectors, fields.OneTermEqualSelector(req.Key, req.Values[0]))
		case selection.NotEquals:
			selectors = append(selectors, fields.OneTermNotEqualSelector(req.Key, req.Values[0]))
		default:
			return nil, fmt.Errorf("unsupported field selector operator: %s", req.Operator)
		}
	}
	return fields.AndSelectors(selectors...), nil
}

// Patch implements store.Store.
func (c *generic) Patch(ctx context.Context, obj store.Object, patch store.Patch, opts ...store.PatchOption) error {
	options := store.PatchOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	preconditions := &storage.Preconditions{}
	if obj.GetUID() != "" {
		preconditions.UID = ptr.To(types.UID(obj.GetUID()))
	}
	updatefunc := func(ctx context.Context, current *store.Unstructured) (newObj store.Object, err error) {
		patchdata, err := patch.Data(obj)
		if err != nil {
			return nil, err
		}
		if err := applyPatch(current, patch.Type(), patchdata); err != nil {
			return nil, err
		}
		return current, nil
	}
	return c.update(ctx, obj, preconditions, updatefunc)
}

func applyPatch(to any, patchtype store.PatchType, patchdata []byte) error {
	switch patchtype {
	case store.PatchTypeJSONPatch:
		return store.JsonPatchObject(to, patchdata)
	case store.PatchTypeMergePatch:
		return store.JsonMergePatchObject(to, patchdata)
	default:
		return fmt.Errorf("unsupported patch type: %s", patchtype)
	}
}

// Scope implements store.Store.
func (c *generic) Scope(scope ...store.Scope) store.Store {
	return &generic{
		core:   c.core,
		scopes: append(c.scopes, scope...),
	}
}

var errShouldDelete = fmt.Errorf("should delete")

// Update implements store.Store.
func (c *generic) Update(ctx context.Context, obj store.Object, opts ...store.UpdateOption) error {
	options := store.UpdateOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	updatefunc := func(ctx context.Context, oldObj *store.Unstructured) (store.Object, error) {
		return obj, nil
	}
	preconditions := &storage.Preconditions{}
	if obj.GetUID() != "" {
		preconditions.UID = ptr.To(types.UID(obj.GetUID()))
	}
	return c.update(ctx, obj, preconditions, updatefunc)
}

func (c *generic) update(ctx context.Context, obj store.Object, preconditions *storage.Preconditions, updatefunc updatFunc) error {
	return c.core.update(ctx, c.scopes, obj, preconditions, updatefunc, true)
}

// Status implements store.Store.
func (c *generic) Status() store.StatusStorage {
	return &status{core: c.core, scopes: c.scopes}
}

var _ store.StatusStorage = &status{}

type status struct {
	core   *core
	scopes []store.Scope
}

// Patch implements store.StatusStorage.
func (s *status) Patch(ctx context.Context, obj store.Object, patch store.Patch, opts ...store.PatchOption) error {
	options := store.PatchOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	preconditions := &storage.Preconditions{}
	if obj.GetUID() != "" {
		preconditions.UID = ptr.To(types.UID(obj.GetUID()))
	}
	updatefunc := func(ctx context.Context, current *store.Unstructured) (newObj store.Object, err error) {
		patchdata, err := patch.Data(obj)
		if err != nil {
			return nil, err
		}
		if err := applyPatch(current, patch.Type(), patchdata); err != nil {
			return nil, err
		}
		return current, nil
	}
	return s.update(ctx, obj, preconditions, updatefunc)
}

func (s *status) update(ctx context.Context, obj store.Object, preconditions *storage.Preconditions, updatefunc updatFunc) error {
	return s.core.update(ctx, s.scopes, obj, preconditions, updatefunc, false)
}

// Update implements store.StatusStorage.
func (s *status) Update(ctx context.Context, obj store.Object, opts ...store.UpdateOption) error {
	options := store.UpdateOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	preconditions := &storage.Preconditions{}
	if obj.GetUID() != "" {
		preconditions.UID = ptr.To(types.UID(obj.GetUID()))
	}
	if rev := obj.GetResourceVersion(); rev != 0 {
		preconditions.ResourceVersion = ptr.To(strconv.FormatInt(rev, 10))
	}
	updatefunc := func(ctx context.Context, oldObj *store.Unstructured) (store.Object, error) {
		return obj, nil
	}
	return s.update(ctx, obj, preconditions, updatefunc)
}

type ResourceFieldsMap map[string][]string

type core struct {
	resources      map[string]*db
	resourcesLock  sync.RWMutex
	storagePrefix  string
	cli            *clientv3.Client
	resourceFields ResourceFieldsMap
}

func (c *core) on(ctx context.Context, example any, fn func(ctx context.Context, db *db) error) error {
	if err := c.validateObject(example); err != nil {
		return err
	}
	resource, err := store.GetResource(example)
	if err != nil {
		return err
	}
	return convertError(fn(ctx, c.getResource(resource)))
}

func convertError(err error) error {
	if err == nil {
		return nil
	}
	if statusErr, ok := err.(*apierrors.StatusError); ok {
		return &errors.Status{
			Status:  statusErr.ErrStatus.Status,
			Code:    statusErr.ErrStatus.Code,
			Message: statusErr.ErrStatus.Message,
			Reason:  errors.StatusReason(statusErr.ErrStatus.Reason),
		}
	}
	return err
}

type updatFunc func(ctx context.Context, current *store.Unstructured) (newObj store.Object, err error)

func (c *core) update(ctx context.Context, scopes []store.Scope, obj store.Object, predicate *storage.Preconditions, fn updatFunc, ignoreStatus bool) error {
	return c.on(ctx, obj, func(ctx context.Context, db *db) error {
		out := &unstructured.Unstructured{}
		key := getObjectKey(scopes, db.resource.String(), obj.GetName())
		err := db.storage.GuaranteedUpdate(ctx, key, out, false, predicate, func(input runtime.Object, res storage.ResponseMeta) (output runtime.Object, ttl *uint64, err error) {
			current, ok := input.(*unstructured.Unstructured)
			if !ok {
				return nil, nil, fmt.Errorf("unexpected object type: %T", input)
			}
			// backup fields
			statusfield, _, _ := unstructured.NestedFieldNoCopy(current.Object, UnstructuredObjectField, "status")
			deletionTimestamp := current.GetDeletionTimestamp()
			unsobj := &store.Unstructured{}
			if err := ConvertFromUnstructured(current, unsobj); err != nil {
				return nil, nil, err
			}
			unsobjchanged, err := fn(ctx, unsobj)
			if err != nil {
				return nil, nil, err
			}
			newuns, err := ConvertToUnstructured(unsobjchanged)
			if err != nil {
				return nil, nil, err
			}
			// restore ignored fields
			if ignoreStatus {
				// keep status field
				unstructured.SetNestedField(newuns.Object, statusfield, UnstructuredObjectField, "status")
			}
			// do not allow deletionTimestamp to be changed if it is already set
			if deletionTimestamp != nil {
				newuns.SetDeletionTimestamp(deletionTimestamp)
			}
			if genericregistry.ShouldDeleteDuringUpdate(ctx, key, newuns, current) {
				return newuns, nil, errShouldDelete
			}
			return newuns, nil, nil
		}, nil)
		if err != nil {
			if err == errShouldDelete {
				// Using the rest.ValidateAllObjectFunc because the request is an UPDATE request and has already passed the admission for the UPDATE verb.
				if err := db.storage.Delete(ctx, key, out, predicate, rest.ValidateAllObjectFunc, nil); err != nil {
					// Deletion is racy, i.e., there could be multiple update
					// requests to remove all finalizers from the object, so we
					// ignore the NotFound error.
					if !storage.IsNotFound(err) {
						err = storeerr.InterpretDeleteError(err, db.resource, obj.GetName())
						return err
					}
					// pass
				}
				// pass
			} else {
				err = storeerr.InterpretUpdateError(err, db.resource, obj.GetName())
				return err
			}
		}
		ConvertFromUnstructured(out, obj)
		return nil
	})
}

func (e *core) validateObject(obj any) error {
	if obj == nil {
		return errors.NewBadRequest("object is nil")
	}
	if _, err := store.EnforcePtr(obj); err != nil {
		return errors.NewBadRequest(fmt.Sprintf("object must be a pointer: %v", err))
	}
	return nil
}

func (c *core) getResource(resource string) *db {
	c.resourcesLock.Lock()
	defer c.resourcesLock.Unlock()
	resourceStorage, ok := c.resources[resource]
	if !ok {
		fields := c.resourceFields[resource]
		groupResource := schema.GroupResource{Resource: resource}
		newresourceStorage, err := newResourceStorage(c.cli, c.storagePrefix, groupResource, fields)
		if err != nil {
			return nil
		}
		c.resources[resource] = newresourceStorage
		resourceStorage = newresourceStorage
	}
	return resourceStorage
}

func getUnstructuredFieldIndex(uns *unstructured.Unstructured, field string) (string, error) {
	val, ok, err := unstructured.NestedFieldNoCopy(uns.Object, append([]string{UnstructuredObjectField}, strings.Split(field, ".")...)...)
	if err != nil {
		return "", fmt.Errorf("error getting field %s: %v", field, err)
	}
	if !ok {
		return "", nil
	}
	switch v := val.(type) {
	case string:
		return v, nil
	case bool:
		return strconv.FormatBool(v), nil
	case int:
		return strconv.Itoa(v), nil
	case int32:
		return strconv.FormatInt(int64(v), 10), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

type db struct {
	storage  storage.Interface
	resource schema.GroupResource
}

func newResourceStorage(cli *clientv3.Client, prefix string, groupResource schema.GroupResource, indexfields []string) (*db, error) {
	transformer := identity.NewEncryptCheckTransformer()
	leaseConfig := etcd3.NewDefaultLeaseManagerConfig()
	newFunc := func() runtime.Object { return &unstructured.Unstructured{} }
	newListFunc := func() runtime.Object { return &unstructured.UnstructuredList{} }

	typer := UnstructuredObjectTyper{}
	creater := UnstructuredCreator{}
	codec := json.NewSerializerWithOptions(DefaultMetaFactory, creater, typer,
		json.SerializerOptions{Yaml: false, Pretty: false, Strict: false})

	// codec := SimpleJsonCodec{}
	resourcePrefix := "/" + groupResource.String()
	etcd3storage := etcd3.New(cli, codec, newFunc, newListFunc, prefix, resourcePrefix, groupResource, transformer, leaseConfig)
	indexers := IndexerFromFields(indexfields)
	cacherConfig := cacherstorage.Config{
		Storage:        etcd3storage,
		Versioner:      storage.APIObjectVersioner{},
		GroupResource:  groupResource,
		ResourcePrefix: resourcePrefix,
		KeyFunc:        ScopesObjectKeyFunc,
		NewFunc:        newFunc,
		NewListFunc:    newListFunc,
		GetAttrsFunc:   GetAttrsFuncfunc(indexfields),
		Codec:          codec,
		Indexers:       &indexers,
	}
	cacher, err := cacherstorage.NewCacherFromConfig(cacherConfig)
	if err != nil {
		return nil, err
	}
	return &db{
		storage:  cacher,
		resource: groupResource,
	}, nil
}

func IndexerFromFields(fields []string) cache.Indexers {
	indexers := cache.Indexers{}
	for _, field := range fields {
		indexers[field] = func(obj interface{}) ([]string, error) {
			uns, ok := obj.(*unstructured.Unstructured)
			if !ok {
				return nil, fmt.Errorf("object is not an unstructured.Unstructured")
			}
			val, err := getUnstructuredFieldIndex(uns, field)
			if err != nil {
				return nil, err
			}
			return []string{val}, nil
		}
	}
	return indexers
}

func GetAttrsFuncfunc(indexfields []string) func(obj runtime.Object) (labels.Set, fields.Set, error) {
	return func(obj runtime.Object) (labels.Set, fields.Set, error) {
		uns, ok := obj.(*unstructured.Unstructured)
		if !ok {
			return nil, nil, fmt.Errorf("unexpected object type: %T", obj)
		}
		sFields := fields.Set{
			"metadata.name": uns.GetName(),
			"name":          uns.GetName(),
		}
		for _, fname := range indexfields {
			valstr, err := getUnstructuredFieldIndex(uns, fname)
			if err != nil {
				return nil, nil, err
			}
			sFields[fname] = valstr
		}
		return uns.GetLabels(), sFields, nil
	}
}

func ScopesObjectKeyFunc(obj runtime.Object) (string, error) {
	uns, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return "", fmt.Errorf("unexpected object type: %T", obj)
	}
	scopes, err := ParseScopes(uns)
	if err != nil {
		return "", err
	}
	return getObjectKey(scopes, uns.GetKind(), uns.GetName()), nil
}

const UnstructuredObjectField = "data"

func ConvertToUnstructured(obj store.Object) (*unstructured.Unstructured, error) {
	uns := &unstructured.Unstructured{Object: map[string]any{}}
	// map metadata to unstructured's metadata
	uns.SetAPIVersion("v1")
	uns.SetKind(obj.GetResource())
	uns.SetName(obj.GetName())
	uns.SetLabels(obj.GetLabels())
	uns.SetAnnotations(obj.GetAnnotations())
	uns.SetResourceVersion(strconv.FormatInt(obj.GetResourceVersion(), 10))
	uns.SetFinalizers(obj.GetFinalizers())
	uns.SetUID(types.UID(obj.GetUID()))
	uns.SetCreationTimestamp(obj.GetCreationTimestamp())
	uns.SetDeletionTimestamp(obj.GetDeletionTimestamp())
	// store values in "data" field
	obj.SetResourceVersion(0) // reset resource version before saving
	values, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}
	uns.Object[UnstructuredObjectField] = values
	return uns, nil
}

func ConvertFromUnstructured(uns *unstructured.Unstructured, obj store.Object) error {
	datafield, ok := uns.Object[UnstructuredObjectField].(map[string]any)
	if !ok {
		datafield = map[string]any{}
	}
	// decode data field
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(datafield, obj); err != nil {
		return err
	}
	// restore metadata
	obj.SetName(uns.GetName())
	obj.SetResource(uns.GetKind())
	obj.SetLabels(uns.GetLabels())
	obj.SetAnnotations(uns.GetAnnotations())
	obj.SetFinalizers(uns.GetFinalizers())
	obj.SetUID(string(uns.GetUID()))
	obj.SetCreationTimestamp(uns.GetCreationTimestamp())
	obj.SetDeletionTimestamp(uns.GetDeletionTimestamp())
	rev, _ := strconv.ParseInt(uns.GetResourceVersion(), 10, 64)
	obj.SetResourceVersion(rev)
	return nil
}

func getObjectKey(scopes []store.Scope, resource, name string) string {
	key := "/" + resource
	for _, scope := range scopes {
		key += "/" + scope.Resource + "/" + scope.Name
	}
	return key + "/" + name
}

func getlistkey(scopes []store.Scope, resource string) string {
	key := "/" + resource
	for _, scope := range scopes {
		key += "/" + scope.Resource + "/" + scope.Name
	}
	return key + "/"
}

func FilterByScopes(list []unstructured.Unstructured, scopes []store.Scope) []unstructured.Unstructured {
	filtered := make([]unstructured.Unstructured, 0, len(list))
	for _, uns := range list {
		thisscopes, err := ParseScopes(&uns)
		if err != nil {
			continue
		}
		if store.ScopesEquals(thisscopes, scopes) {
			filtered = append(filtered, uns)
		}
	}
	return filtered
}

func SortUnstructuredList(list []unstructured.Unstructured, by string) {
	slices.SortFunc(list, func(a, b unstructured.Unstructured) int {
		switch by {
		case "time":
			return a.GetCreationTimestamp().Time.Compare(b.GetCreationTimestamp().Time)
		case "time-", "": // default sort by time desc
			return b.GetCreationTimestamp().Time.Compare(a.GetCreationTimestamp().Time)
		case "name":
			return strings.Compare(a.GetName(), b.GetName())
		case "name-":
			return strings.Compare(b.GetName(), a.GetName())
		default:
			return 0
		}
	})
}

func PageUnstructuredList(list []unstructured.Unstructured, page, size int) []unstructured.Unstructured {
	if page == 0 && size == 0 {
		return list
	}
	if page == 0 {
		page = 1
	}
	total := len(list)
	startIdx := (page - 1) * size
	endIdx := startIdx + size
	if startIdx > total {
		startIdx = 0
		endIdx = 0
	}
	if endIdx > total {
		endIdx = total
	}
	list = list[startIdx:endIdx]
	return list
}
