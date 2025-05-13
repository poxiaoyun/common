package controller

import (
	"context"
	"encoding/json"
	"reflect"
	"time"

	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/store"
)

type Reconciler[T store.Object] interface {
	Sync(ctx context.Context, obj T) (Result, error)
	Remove(ctx context.Context, obj T) (Result, error)
}

type SimpleResourceReconciler[T store.Object] interface {
	Sync(ctx context.Context, obj T) error
	Remove(ctx context.Context, obj T) error
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
	usePatchStatus   bool
}

// WithFinalizer auto add finalizer to the object if it is not exist.
// it also remove the finalizer when the object is deleted and function Remove() return nil.
func WithFinalizer(finalizer string) BetterReconcilerOption {
	return func(o *BetterReconcilerOptions) {
		o.finalizer = finalizer
	}
}

// WithPatchStatus use patch to update the status instead of update.
func WithPatchStatus() BetterReconcilerOption {
	return func(o *BetterReconcilerOptions) {
		o.usePatchStatus = true
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

func (r *BetterReconciler[T]) Initialize(ctx context.Context) error {
	if init, ok := r.Reconciler.(InitializeReconciler); ok {
		return init.Initialize(ctx)
	}
	return nil
}

// nolint: funlen,gocognit
func (r *BetterReconciler[T]) Reconcile(ctx context.Context, key ScopedKey) (Result, error) {
	log := log.FromContext(ctx).WithValues("key", key)

	log.Info("start reconcile")
	defer log.Info("finish reconcile")

	obj, _ := reflect.New(reflect.TypeOf(*new(T)).Elem()).Interface().(T)

	condStorage := r.Client.Scope(key.Scopes()...)
	if err := condStorage.Get(ctx, key.Name, obj); err != nil {
		// if object not found , just ignore it
		// if a post handler needed after object deleted, consider to add a finalizer instead
		if errors.IsNotFound(err) {
			return Result{}, nil
		}
		return Result{}, err
	}

	finalizer := r.Options.finalizer
	if obj.GetDeletionTimestamp() != nil {
		log.Info("is being deleted")
		if finalizer != "" && !ContainsFinalizer(obj, finalizer) {
			return Result{}, nil
		}
		// remove
		result, err := r.processWithPostFunc(ctx, condStorage, obj, r.Reconciler.Remove)
		if err != nil {
			return result, err
		}
		if finalizer != "" && RemoveFinalizer(obj, finalizer) {
			if err := condStorage.Status().Update(ctx, obj); err != nil {
				return Result{}, err
			}
		}
		return result, nil
	}
	if finalizer != "" && AddFinalizer(obj, finalizer) {
		if err := condStorage.Status().Update(ctx, obj); err != nil {
			return Result{}, err
		}
	}
	// sync
	return r.processWithPostFunc(ctx, condStorage, obj, r.Reconciler.Sync)
}

func (r *BetterReconciler[T]) processWithPostFunc(ctx context.Context, condStorage store.Store, obj T, fun TypedReconcilerFunc[T]) (Result, error) {
	log := log.FromContext(ctx)
	original := DeepCopyObject(obj)

	result, funcerr := fun(ctx, obj)
	if funcerr != nil {
		r.setStatusMessage(obj, funcerr)
		if !reflect.DeepEqual(original, obj) {
			if r.Options.usePatchStatus {
				if updateerr := condStorage.Status().Patch(ctx, obj, store.MergePatchFrom(original)); updateerr != nil {
					log.Error(updateerr, "unable to patch status")
				}
			} else {
				if updateerr := condStorage.Status().Update(ctx, obj); updateerr != nil {
					log.Error(updateerr, "unable to update status")
				}
			}
		}
	} else {
		// success reconciled
		// clear message
		r.setStatusMessage(obj, nil)
		if !reflect.DeepEqual(original, obj) {
			if r.Options.usePatchStatus {
				if updateerr := condStorage.Status().Patch(ctx, obj, store.MergePatchFrom(original)); updateerr != nil {
					return Result{}, updateerr
				}
			} else {
				if updateerr := condStorage.Status().Update(ctx, obj); updateerr != nil {
					return Result{}, updateerr
				}
			}
		}
		if !result.Requeue && r.Options.requeueOnSuccess != 0 {
			return Result{Requeue: true, RequeueAfter: r.Options.requeueOnSuccess}, nil
		}
	}
	return result, funcerr
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
