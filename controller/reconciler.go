package controller

import (
	"context"
	"encoding/json"
	"reflect"
	"time"

	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/store"
)

type Reconciler[T store.Object] interface {
	Sync(ctx context.Context, store store.Store, obj T) error
	Remove(ctx context.Context, store store.Store, obj T) error
}

// BetterReconciler wraps the reconciler with some features.
// - reversion check,see below.
// - auto update the status after sync if the object is changed.
// - auto add/remove finalizer.
type BetterReconciler[T store.Object] struct {
	Options    BetterReconcilerOptions
	Client     store.Store
	Reconciler Reconciler[T]
}

type BetterReconcilerOptions struct {
	finalizer        string
	requeueOnSuccess time.Duration
	autosetStatus    bool
}

// WithFinalizer auto add finalizer to the object if it is not exist.
// it also remove the finalizer when the object is deleted and function Remove() return nil.
func WithFinalizer(finalizer string) BetterReconcilerOption {
	return func(o *BetterReconcilerOptions) {
		o.finalizer = finalizer
	}
}

// WithRequeueOnSuccess is a option to requeue the successed request.
// it similar with informers resync period, but informer resync perid is not configurable for every reconciler.
func WithRequeueOnSuccess(duration time.Duration) BetterReconcilerOption {
	return func(o *BetterReconcilerOptions) {
		o.requeueOnSuccess = duration
	}
}

func WithAutosetStatus() BetterReconcilerOption {
	return func(o *BetterReconcilerOptions) {
		o.autosetStatus = true
	}
}

type BetterReconcilerOption func(*BetterReconcilerOptions)

func NewBetterReconciler[T store.Object](r Reconciler[T], cli store.Store, options ...BetterReconcilerOption) *BetterReconciler[T] {
	opts := &BetterReconcilerOptions{}
	for _, opt := range options {
		opt(opts)
	}
	return &BetterReconciler[T]{Options: *opts, Reconciler: r, Client: cli}
}

// nolint: funlen,gocognit
func (r *BetterReconciler[T]) Reconcile(ctx context.Context, key *ScopedKey) error {
	log := log.FromContext(ctx)

	log.Info("start reconcile")
	defer log.Info("finish reconcile")

	obj, _ := reflect.New(reflect.TypeOf(*new(T)).Elem()).Interface().(T)

	condStorage := r.Client.Scope(key.Scopes...)
	if err := condStorage.Get(ctx, key.Name, obj); err != nil {
		log.Error(err, "unable to fetch")
		// if object not found , just ignore it
		// if a post handler needed after object deleted, consider to add a finalizer instead
		return store.IgnoreNotFound(err)
	}

	finalizer := r.Options.finalizer
	if obj.GetDeletionTimestamp() != nil {
		log.Info("is being deleted")
		if finalizer != "" && !ContainsFinalizer(obj, finalizer) {
			if len(obj.GetFinalizers()) == 0 {
				// delete now
				return condStorage.Delete(ctx, obj)
			}
			return nil
		}
		// remove
		if err := r.processWithPostFunc(ctx, condStorage, obj, r.Reconciler.Remove); err != nil {
			return err
		}
		if finalizer != "" {
			RemoveFinalizer(obj, finalizer)
		}
		if err := condStorage.Status().Update(ctx, obj); err != nil {
			return err
		}
		if len(obj.GetFinalizers()) == 0 {
			// delete now
			return condStorage.Delete(ctx, obj)
		}
		return nil
	}
	if finalizer != "" && AddFinalizer(obj, finalizer) {
		if err := condStorage.Status().Update(ctx, obj); err != nil {
			return err
		}
	}
	// sync
	return r.processWithPostFunc(ctx, condStorage, obj, r.Reconciler.Sync)
}

func (r *BetterReconciler[T]) processWithPostFunc(ctx context.Context, condStorage store.Store, obj T, fun func(ctx context.Context, condStorage store.Store, obj T) error) error {
	log := log.FromContext(ctx)
	original := DeepCopyObject(obj)

	funcerr := fun(ctx, condStorage, obj)
	if funcerr != nil {
		r.setStatusMessage(obj, funcerr)
		if !reflect.DeepEqual(original, obj) {
			if updateerr := condStorage.Status().Update(ctx, obj); updateerr != nil {
				log.Error(updateerr, "unable to update status")
			}
		}
		return funcerr
	}
	// success reconciled
	// clear message
	r.setStatusMessage(obj, nil)
	if !reflect.DeepEqual(original, obj) {
		if updateerr := condStorage.Status().Update(ctx, obj); updateerr != nil {
			log.Error(updateerr, "unable to update status")
			return updateerr
		}
	}
	if r.Options.requeueOnSuccess > 0 {
		log.Info("requeue after success", "duration", r.Options.requeueOnSuccess)
		return WithReQueue(r.Options.requeueOnSuccess, funcerr)
	}
	return funcerr
}

func NewObject[T any](t reflect.Type) T {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
		return reflect.New(t).Interface().(T)
	}
	return reflect.New(t).Elem().Interface().(T)
}

func DeepCopyObject[T store.Object](obj T) T {
	newval := NewObject[T](reflect.TypeOf(obj))
	data, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}
	if err := json.Unmarshal(data, newval); err != nil {
		panic(err)
	}
	return newval
}

func (r *BetterReconciler[T]) setStatusMessage(obj T, err error) {
	if !r.Options.autosetStatus {
		return
	}
	v := reflect.ValueOf(obj)
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}

	statusv := v.FieldByName("Status")
	if !statusv.IsValid() || statusv.Kind() != reflect.Struct {
		return
	}
	msgv := statusv.FieldByName("Message")
	if !msgv.IsValid() || msgv.Kind() != reflect.String {
		return
	}
	if msgv.CanSet() {
		if err != nil {
			msgv.SetString(err.Error())
		} else {
			msgv.SetString("")
		}
	}
}

// ContainsFinalizer checks an Object that the provided finalizer is present.
func ContainsFinalizer(o store.Object, finalizer string) bool {
	f := o.GetFinalizers()
	for _, e := range f {
		if e == finalizer {
			return true
		}
	}
	return false
}

// RemoveFinalizer accepts an Object and removes the provided finalizer if present.
// It returns an indication of whether it updated the object's list of finalizers.
func RemoveFinalizer(o store.Object, finalizer string) (finalizersUpdated bool) {
	f := o.GetFinalizers()
	length := len(f)

	index := 0
	for i := 0; i < length; i++ {
		if f[i] == finalizer {
			continue
		}
		f[index] = f[i]
		index++
	}
	o.SetFinalizers(f[:index])
	return length != index
}

func AddFinalizer(o store.Object, finalizer string) (finalizersUpdated bool) {
	f := o.GetFinalizers()
	for _, e := range f {
		if e == finalizer {
			return false
		}
	}
	o.SetFinalizers(append(f, finalizer))
	return true
}
