package etcd

import (
	"bytes"
	"context"
	stderrors "errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	grpcprom "github.com/grpc-ecosystem/go-grpc-prometheus"
	"go.etcd.io/etcd/client/pkg/v3/transport"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/kubernetes"
	"google.golang.org/grpc"
	"k8s.io/utils/ptr"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/store"
)

var ErrResourceVersionSetOnCreate = stderrors.New("resourceVersion should not be set on objects to be created")

func NewDefaultOptions() *Options {
	return &Options{
		Servers:   []string{"http://127.0.0.1:2379"},
		KeyPrefix: "/core",
	}
}

type Options struct {
	Servers       []string `json:"servers,omitempty"`
	Username      string   `json:"username,omitempty"`
	Password      string   `json:"password,omitempty"`
	KeyFile       string   `json:"keyFile,omitempty"`
	CertFile      string   `json:"certFile,omitempty"`
	TrustedCAFile string   `json:"trustedCAFile,omitempty"`
	KeyPrefix     string   `json:"keyPrefix,omitempty"`
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
)

func NewETCD3Client(ctx context.Context, c *Options) (*kubernetes.Client, error) {
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
	dialOptions := []grpc.DialOption{
		// use chained interceptors so that the default (retry and backoff) interceptors are added.
		// otherwise they will be overwritten by the metric interceptor.
		//
		// these optional interceptors will be placed after the default ones.
		// which seems to be what we want as the metrics will be collected on each attempt (retry)
		grpc.WithChainUnaryInterceptor(grpcprom.UnaryClientInterceptor),
		grpc.WithChainStreamInterceptor(grpcprom.StreamClientInterceptor),
	}
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
	return kubernetes.New(cfg)
}

func NewEtcdStore(ctx context.Context, c *Options) (*EtcdStore, error) {
	client, err := NewETCD3Client(ctx, c)
	if err != nil {
		return nil, err
	}
	// check if the server is up
	timeouts, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	log := log.FromContext(ctx)
	log.Info("checking etcd server health")
	if _, err := client.Get(timeouts, "health", kubernetes.GetOptions{}); err != nil {
		return nil, fmt.Errorf("etcd server is not reachable: %v", err)
	}
	return NewEtcdStoreFromClient(client, c.KeyPrefix), nil
}

func NewEtcdStoreFromClient(client *kubernetes.Client, keyPrefix string) *EtcdStore {
	return &EtcdStore{core: newEtcdStoreCore(client, keyPrefix)}
}

func newEtcdStoreCore(client *kubernetes.Client, keyPrefix string) *etcdStoreCore {
	return &etcdStoreCore{
		client:     client,
		KeyPrefix:  keyPrefix,
		serializer: &store.JSONSerializer{},
		leases:     newEtcd3LeaseManager(client, 0, 0, 0),
	}
}

var _ store.Store = &EtcdStore{}

type EtcdStore struct {
	scopes []store.Scope
	core   *etcdStoreCore
}

// PatchBatch implements store.Store.
func (e *EtcdStore) PatchBatch(ctx context.Context, obj store.ObjectList, patch store.PatchBatch, opts ...store.PatchBatchOption) error {
	return errors.NewNotImplemented("etcd does not support batch patch")
}

// DeleteBatch implements store.Store.
func (e *EtcdStore) DeleteBatch(ctx context.Context, obj store.ObjectList, opts ...store.DeleteBatchOption) error {
	return errors.NewNotImplemented("etcd does not support delete batch")
}

// Count implements Store.
func (e *EtcdStore) Count(ctx context.Context, obj store.Object, opts ...store.CountOption) (int, error) {
	resource, err := store.GetResource(obj)
	if err != nil {
		return 0, err
	}
	preparedKey := e.core.getkey(e.scopes, resource, "")

	getResp, err := e.core.client.KV.Get(ctx,
		preparedKey,
		clientv3.WithRange(clientv3.GetPrefixRangeEnd(preparedKey)),
		clientv3.WithCountOnly())
	if err != nil {
		return 0, errors.NewInternalError(err)
	}
	return int(getResp.Count), nil
}

// Create implements Store.
func (e *EtcdStore) Create(ctx context.Context, obj store.Object, opts ...store.CreateOption) error {
	resource, err := store.GetResource(obj)
	if err != nil {
		return err
	}
	creatoptions := &store.CreateOptions{}
	for _, opt := range opts {
		opt(creatoptions)
	}
	if err := e.core.validateObject(obj); err != nil {
		return err
	}
	if obj.GetName() == "" {
		return errors.NewBadRequest("name is required")
	}
	if obj.GetResourceVersion() != 0 {
		return errors.NewInvalid(resource, obj.GetName(), ErrResourceVersionSetOnCreate)
	}
	obj.SetUID(uuid.New().String())
	obj.SetCreationTimestamp(store.Now())
	obj.SetScopes(e.scopes)
	obj.SetResource(resource)

	preparedKey := e.core.getkey(e.scopes, resource, obj.GetName())

	data, err := e.core.serializer.Encode(obj)
	if err != nil {
		return err
	}
	putopts, err := e.core.ttlOpts(ctx, int64(creatoptions.TTL.Seconds()))
	if err != nil {
		return err
	}
	txnResp, err := e.core.client.KV.Txn(ctx).If(
		keyHasRevision(preparedKey, 0), // key does not exist
	).Then(
		clientv3.OpPut(preparedKey, string(data), putopts...),
	).Commit()
	if err != nil {
		return errors.NewInternalError(err)
	}
	if !txnResp.Succeeded {
		return errors.NewAlreadyExists(resource, obj.GetName())
	}

	putResp := txnResp.Responses[0].GetResponsePut()
	resourceVersion := putResp.Header.Revision
	// decode once
	if err := e.core.serializer.Decode(data, obj); err != nil {
		return errors.NewInternalError(err)
	}
	obj.SetResourceVersion(resourceVersion)
	return nil
}

func keyHasRevision(key string, rev int64) clientv3.Cmp {
	return clientv3.Compare(clientv3.ModRevision(key), "=", rev)
}

// Delete implements Store.
func (e *EtcdStore) Delete(ctx context.Context, obj store.Object, opts ...store.DeleteOption) error {
	deleteoptions := &store.DeleteOptions{
		// Default delete forgroud
		PropagationPolicy: ptr.To(store.DeletePropagationForeground),
	}
	for _, opt := range opts {
		opt(deleteoptions)
	}
	if err := e.core.validateObject(obj); err != nil {
		return err
	}
	if obj.GetName() == "" {
		return errors.NewBadRequest("name is required")
	}

	if obj.GetDeletionTimestamp() == nil {
		obj.SetDeletionTimestamp(ptr.To(store.Now()))
	}
	// update finalizers ac
	if deleteoptions.PropagationPolicy != nil {
		gcFinalizers := []string{}
		switch *deleteoptions.PropagationPolicy {
		case store.DeletePropagationForeground:
			gcFinalizers = append(gcFinalizers, store.FinalizerDeleteDependents)
		}
		nogcFinalizers := slices.DeleteFunc(obj.GetFinalizers(), func(finalizer string) bool {
			return finalizer == store.FinalizerDeleteDependents
		})
		obj.SetFinalizers(append(nogcFinalizers, gcFinalizers...))
	}
	if len(obj.GetFinalizers()) != 0 {
		updatefunc := func(current store.Object) (store.Object, error) {
			// if rev := deleteoptions.ResourceVersion; rev > 0 {
			// 	if current.GetResourceVersion() != rev {
			// 		return nil, errors.NewConflict(resource, obj.GetName(),
			// 			fmt.Errorf("resourceVersion %d does not match", rev))
			// 	}
			// }
			current.SetDeletionTimestamp(obj.GetDeletionTimestamp())
			current.SetFinalizers(obj.GetFinalizers())
			return current, nil
		}
		return e.core.tryUpdate(ctx, e.scopes, obj, updatefunc, tryUpdateOptions{UseUnstructured: true})
	}
	// return e.core.directDelete(ctx, e.scopes, obj, deleteoptions.ResourceVersion)
	return e.core.directDelete(ctx, e.scopes, obj, 0)
}

// Get implements Store.
func (e *EtcdStore) Get(ctx context.Context, name string, obj store.Object, opts ...store.GetOption) error {
	resource, err := store.GetResource(obj)
	if err != nil {
		return err
	}
	options := &store.GetOptions{}
	for _, opt := range opts {
		opt(options)
	}
	if err := e.core.validateObject(obj); err != nil {
		return err
	}
	preparedKey := e.core.getkey(e.scopes, resource, name)
	_, err = e.core.getCurrent(ctx, preparedKey, obj, obj.GetResourceVersion())
	return err
}

const maxLimit = 10000

// List implements Store.
func (e *EtcdStore) List(ctx context.Context, list store.ObjectList, opts ...store.ListOption) error {
	resource, err := store.GetResource(list)
	if err != nil {
		return err
	}
	options := &store.ListOptions{}
	for _, opt := range opts {
		opt(options)
	}
	if err := e.core.validateObjectList(list); err != nil {
		return err
	}
	v, newItemFunc, err := store.NewItemFuncFromList(list)
	if err != nil {
		return err
	}
	preparedKey := e.core.getlistkey(e.scopes, resource)

	getoptions := make([]clientv3.OpOption, 0, 2)
	rangeEnd := clientv3.GetPrefixRangeEnd(preparedKey)
	getoptions = append(getoptions, clientv3.WithRange(rangeEnd))

	withRev := options.ResourceVersion
	if withRev != nil {
		getoptions = append(getoptions, clientv3.WithRev(*withRev))
	}
	limit := options.Size
	skip := 0
	if options.Page-1 > 0 && limit > 0 {
		skip = options.Page * limit
	}
	// clean existing items
	v.SetZero()
	for {
		if limit > 0 {
			// we want to fetch skip + limit items to skip the first `skip` items
			getoptions = append(getoptions, clientv3.WithLimit(int64(limit+skip)))
		}
		getResp, err := e.core.client.KV.Get(ctx, preparedKey, getoptions...)
		if err != nil {
			return interpretListError(list.GetResource(), err)
		}
		hasMore := getResp.More
		if len(getResp.Kvs) == 0 && hasMore {
			return errors.NewInternalError(fmt.Errorf("etcd returned no keys but more is true"))
		}
		if withRev == nil {
			withRev = ptr.To(getResp.Header.Revision)
			getoptions = append(getoptions, clientv3.WithRev(*withRev))
		}
		store.GrowSlice(v, len(getResp.Kvs))
		for _, kv := range getResp.Kvs {
			// has subresources
			if !options.IncludeSubScopes {
				if index := bytes.Index(kv.Key[len(preparedKey):], []byte("/")); index != -1 {
					continue
				}
			}
			// Check if the request has already timed out before decode object
			select {
			case <-ctx.Done():
				// parent context is canceled or timed out, no point in continuing
				return errors.NewBadRequest("request timeout")
			default:
			}
			if skip > 0 {
				skip--
				continue
			}
			obj := newItemFunc()
			if err := e.core.serializer.Decode(kv.Value, obj); err != nil {
				return errors.NewInternalError(err)
			}
			obj.SetResourceVersion(kv.ModRevision)

			// check if the object matches the label requirements
			if store.MatchLabelReqirements(obj, options.LabelRequirements) {
				v.Set(reflect.Append(v, reflect.ValueOf(obj).Elem()))
			}
		}
		if !hasMore || limit == 0 {
			break
		}
		if limit > 0 && v.Len() >= limit {
			break
		}
	}
	if v.IsNil() {
		// Ensure that we never return a nil Items pointer in the result for consistency.
		v.Set(reflect.MakeSlice(v.Type(), 0, 0))
	}
	list.SetResourceVersion(ptr.Deref(withRev, 0))
	list.SetScopes(e.scopes)
	return nil
}

// Patch implements Store.
func (e *EtcdStore) Patch(ctx context.Context, obj store.Object, patch store.Patch, opts ...store.PatchOption) error {
	options := &store.PatchOptions{}
	for _, opt := range opts {
		opt(options)
	}
	updatefunc := func(current store.Object) (store.Object, error) {
		// backup status field
		status, hashStatus, err := GetObjectField(obj, "Status")
		if err != nil {
			return nil, errors.NewBadRequest(err.Error())
		}
		// apply patch
		if err := store.ApplyPatch(current, obj, patch); err != nil {
			return nil, err
		}
		// restore status field
		if hashStatus {
			if _, err := SetObjectField(current, "Status", status); err != nil {
				return nil, err
			}
		}
		return current, nil
	}
	return e.core.tryUpdate(ctx, e.scopes, obj, updatefunc, tryUpdateOptions{UseUnstructured: true})
}

// Scope implements Store.
func (e *EtcdStore) Scope(scope ...store.Scope) store.Store {
	return &EtcdStore{core: e.core, scopes: append(e.scopes, scope...)}
}

// Status implements Store.
func (e *EtcdStore) Status() store.StatusStorage {
	return &EtcdStatusStore{core: e.core, scopes: e.scopes}
}

// Update implements Store.
func (e *EtcdStore) Update(ctx context.Context, obj store.Object, opts ...store.UpdateOption) error {
	options := &store.UpdateOptions{}
	for _, opt := range opts {
		opt(options)
	}
	updatefunc := func(current store.Object) (store.Object, error) {
		if resourceVersion := obj.GetResourceVersion(); resourceVersion != 0 {
			if resourceVersion != current.GetResourceVersion() {
				return nil, errors.NewConflict(current.GetResource(), obj.GetName(),
					fmt.Errorf("resourceVersion %d does not match", resourceVersion))
			}
		}
		// update the object expect status
		if err := CopyField(obj, current, "Status"); err != nil {
			return nil, err
		}
		return obj, nil
	}
	return e.core.tryUpdate(ctx, e.scopes, obj, updatefunc, tryUpdateOptions{TTL: int64(options.TTL.Seconds())})
}

var _ store.StatusStorage = &EtcdStatusStore{}

type EtcdStatusStore struct {
	core   *etcdStoreCore
	scopes []store.Scope
}

// Patch implements StatusStorage.
func (e *EtcdStatusStore) Patch(ctx context.Context, obj store.Object, patch store.Patch, opts ...store.PatchOption) error {
	options := &store.PatchOptions{}
	for _, opt := range opts {
		opt(options)
	}
	updatefunc := func(current store.Object) (store.Object, error) {
		// backup spec field
		spec, hashSpec, err := GetObjectField(obj, "Spec")
		if err != nil {
			return nil, errors.NewBadRequest(err.Error())
		}
		//  apply patch
		if err := store.ApplyPatch(current, obj, patch); err != nil {
			return nil, err
		}
		// restore spec field
		if hashSpec {
			if _, err := SetObjectField(current, "Spec", spec); err != nil {
				return nil, err
			}
		}
		return current, nil
	}
	return e.core.tryUpdate(ctx, e.scopes, obj, updatefunc, tryUpdateOptions{UseUnstructured: true})
}

// Update implements StatusStorage.
func (e *EtcdStatusStore) Update(ctx context.Context, obj store.Object, opts ...store.UpdateOption) error {
	resource, err := store.GetResource(obj)
	if err != nil {
		return err
	}
	options := &store.UpdateOptions{}
	for _, opt := range opts {
		opt(options)
	}
	updatefunc := func(current store.Object) (store.Object, error) {
		if resourceVersion := obj.GetResourceVersion(); resourceVersion != 0 {
			if resourceVersion != current.GetResourceVersion() {
				return nil, errors.NewConflict(resource, obj.GetName(),
					fmt.Errorf("resourceVersion %d does not match", resourceVersion))
			}
		}
		// update the object expect spec
		if err := CopyField(obj, current, "Spec"); err != nil {
			return nil, err
		}
		return obj, nil
	}
	return e.core.tryUpdate(ctx, e.scopes, obj, updatefunc, tryUpdateOptions{TTL: int64(options.TTL.Seconds())})
}

type etcdStoreCore struct {
	KeyPrefix  string
	client     *kubernetes.Client
	serializer store.Serializer
	leases     *leaseManager
}

func (s *etcdStoreCore) ttlOpts(ctx context.Context, ttl int64) ([]clientv3.OpOption, error) {
	if ttl == 0 {
		return nil, nil
	}
	id, err := s.leases.GetLease(ctx, ttl)
	if err != nil {
		return nil, err
	}
	return []clientv3.OpOption{clientv3.WithLease(id)}, nil
}

func (e *etcdStoreCore) validateObjectList(obj store.ObjectList) error {
	if obj == nil {
		return errors.NewBadRequest("object list is nil")
	}
	if _, err := store.EnforcePtr(obj); err != nil {
		return errors.NewBadRequest(fmt.Sprintf("object list must be a pointer: %v", err))
	}
	return nil
}

func (e *etcdStoreCore) validateObject(obj store.Object) error {
	if obj == nil {
		return errors.NewBadRequest("object is nil")
	}
	if _, err := store.EnforcePtr(obj); err != nil {
		return errors.NewBadRequest(fmt.Sprintf("object must be a pointer: %v", err))
	}
	return nil
}

func (e *etcdStoreCore) getlistkey(scopes []store.Scope, resource string) string {
	key := e.KeyPrefix + "/" + resource
	for _, scope := range scopes {
		key += "/" + scope.Resource + "/" + scope.Name
	}
	return key + "/"
}

func (e *etcdStoreCore) getkey(scopes []store.Scope, resource, name string) string {
	// merge all scopes into a single key
	key := e.KeyPrefix + "/" + resource
	for _, scope := range scopes {
		key += "/" + scope.Resource + "/" + scope.Name
	}
	key += "/" + name
	return key
}

type tryUpdateOptions struct {
	TTL             int64
	UseUnstructured bool
}

type tryUpdateFunc func(current store.Object) (store.Object, error)

func (e *etcdStoreCore) tryUpdate(ctx context.Context, scopes []store.Scope, obj store.Object, do tryUpdateFunc, options tryUpdateOptions) error {
	v, err := store.EnforcePtr(obj)
	if err != nil {
		return errors.NewBadRequest(fmt.Sprintf("object must be a pointer: %v", err))
	}
	if obj.GetName() == "" {
		return errors.NewBadRequest("name is required")
	}
	name := obj.GetName()
	resource, err := store.GetResource(obj)
	if err != nil {
		return err
	}

	var current store.Object
	if options.UseUnstructured {
		current = &store.Unstructured{}
	} else {
		current = reflect.New(v.Type()).Interface().(store.Object)
	}

	preparedKey := e.getkey(scopes, resource, name)

	currentdata, err := e.getCurrent(ctx, preparedKey, current, 0)
	if err != nil {
		return err
	}
	maxRetries := 5
	for {
		if maxRetries == 0 {
			return errors.NewConflict(resource, name, fmt.Errorf("max retries reached"))
		}
		currentversion := current.GetResourceVersion()
		updated, err := do(current)
		if err != nil {
			return err
		}
		// should set scopes
		updated.SetScopes(scopes)
		updated.SetResource(resource)
		if updated.GetName() != name {
			return errors.NewBadRequest("name cannot be changed")
		}
		updated.SetResourceVersion(0)
		data, err := e.serializer.Encode(updated)
		if err != nil {
			return errors.NewInternalError(err)
		}
		if !bytes.Equal(data, currentdata) {
			putopts, err := e.ttlOpts(ctx, options.TTL)
			if err != nil {
				return err
			}
			txnResp, err := e.client.KV.Txn(ctx).If(
				keyHasRevision(preparedKey, currentversion),
			).Then(
				clientv3.OpPut(preparedKey, string(data), putopts...),
			).Else(
				clientv3.OpGet(preparedKey),
			).Commit()
			if err != nil {
				return errors.NewInternalError(err)
			}
			if !txnResp.Succeeded {
				getResp := (*clientv3.GetResponse)(txnResp.Responses[0].GetResponseRange())
				newcurrent, err := e.decodeGetResp(getResp, current, 0)
				if err != nil {
					return err
				}
				currentdata = newcurrent
				maxRetries--
				continue
			}
			currentversion = txnResp.Responses[0].GetResponsePut().Header.Revision
		}
		if err := e.serializer.Decode(data, obj); err != nil {
			return errors.NewInternalError(err)
		}
		obj.SetResourceVersion(currentversion)
		return nil
	}
}

func (s *etcdStoreCore) getCurrent(ctx context.Context, key string, into store.Object, rev int64) ([]byte, error) {
	getResp, err := s.client.KV.Get(ctx, key)
	if err != nil {
		return nil, errors.NewInternalError(err)
	}
	return s.decodeGetResp(getResp, into, rev)
}

func (s *etcdStoreCore) decodeGetResp(getResp *clientv3.GetResponse, into store.Object, rev int64) ([]byte, error) {
	if len(getResp.Kvs) == 0 {
		return nil, errors.NewNotFound(into.GetResource(), into.GetName())
	}
	kv := getResp.Kvs[0]
	if rev != 0 && kv.ModRevision < rev {
		return nil, errors.NewInvalid(into.GetResource(), into.GetName(),
			fmt.Errorf("resourceVersion %d is newer than current %d", rev, kv.ModRevision))
	}
	if into != nil {
		if err := s.serializer.Decode(kv.Value, into); err != nil {
			return nil, errors.NewInternalError(err)
		}
	}
	into.SetResourceVersion(kv.ModRevision)
	return kv.Value, nil
}

func (e *etcdStoreCore) directDelete(ctx context.Context, scopes []store.Scope, obj store.Object, resourceVersion int64) error {
	resource, err := store.GetResource(obj)
	if err != nil {
		return err
	}

	key := e.getkey(scopes, resource, obj.GetName())

	cmps := []clientv3.Cmp{}
	if resourceVersion != 0 {
		cmps = append(cmps, keyHasRevision(key, resourceVersion))
	}
	txnResp, err := e.client.KV.Txn(ctx).
		If(cmps...).
		Then(clientv3.OpGet(key), clientv3.OpDelete(key)).
		Else(clientv3.OpGet(key)).
		Commit()
	if err != nil {
		return errors.NewInternalError(err)
	}
	if !txnResp.Succeeded {
		getResp := txnResp.Responses[0].GetResponseRange()
		if len(getResp.Kvs) == 0 {
			log.V(4).Info("deletion failed because key not found", "key", key)
			return errors.NewNotFound(resource, obj.GetName())
		}
		log.V(4).Info("deletion failed because resourceVersion does not match", "key", key)
		return errors.NewConflict(resource, obj.GetName(), fmt.Errorf("resourceVersion %d does not match", resourceVersion))
	}
	// always not be empty
	if getResp := txnResp.Responses[0].GetResponseRange(); len(getResp.Kvs) != 0 {
		data := getResp.Kvs[0].Value
		if err := e.serializer.Decode(data, obj); err != nil {
			return errors.NewInternalError(err)
		}
	}
	// fill resourceVersion
	deleteResp := txnResp.Responses[1].GetResponseDeleteRange()
	if header := deleteResp.Header; header != nil {
		obj.SetResourceVersion(header.Revision)
	}
	return nil
}

func GetObjectField(obj any, field string) (any, bool, error) {
	if uns, ok := obj.(*store.Unstructured); ok {
		val, ok := store.GetNestedField(uns.Object, strings.ToLower(field))
		return val, ok, nil
	}
	v, err := store.EnforcePtr(obj)
	if err != nil {
		return nil, false, err
	}
	if v.Kind() != reflect.Struct {
		return nil, false, fmt.Errorf("expected struct, but got %v type", v.Type())
	}
	f := v.FieldByName(field)
	if !f.IsValid() {
		return nil, false, nil
	}
	return f.Interface(), true, nil
}

func SetObjectField(obj any, field string, value any) (bool, error) {
	if uns, ok := obj.(*store.Unstructured); ok {
		store.SetNestedField(uns.Object, value, strings.ToLower(field))
		return true, nil
	}
	v, err := store.EnforcePtr(obj)
	if err != nil {
		return false, err
	}
	if v.Kind() != reflect.Struct {
		return false, fmt.Errorf("expected struct, but got %v type", v.Type())
	}
	f := v.FieldByName(field)
	if !f.IsValid() {
		return false, nil
	}
	if !f.CanSet() {
		return false, fmt.Errorf("field %s is not settable", field)
	}
	f.Set(reflect.ValueOf(value))
	return true, nil
}

func CopyField(to, from any, field string) error {
	oldStatus, ok, err := GetObjectField(from, field)
	if err != nil {
		return err
	}
	if ok {
		if _, err := SetObjectField(to, field, oldStatus); err != nil {
			return err
		}
	}
	return nil
}
