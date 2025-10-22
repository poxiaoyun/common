package controller

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/meta"
	"xiaoshiai.cn/common/store"
)

type OnLeaderElected func(ctx context.Context) error

type LeaderElection interface {
	OnLeader(ctx context.Context, onLeaderElected OnLeaderElected) error
}

type Lease struct {
	store.ObjectMeta     `json:",inline"`
	HolderIdentity       string    `json:"holderIdentity"`
	LeaseDurationSeconds int       `json:"leaseDurationSeconds"`
	AcquireTime          meta.Time `json:"acquireTime"`
	RenewTime            meta.Time `json:"renewTime"`
	LeaderTransitions    int       `json:"leaderTransitions"`
}

func NewStoreLeaderElection(store store.Store, key string, ttl time.Duration) LeaderElection {
	if ttl == 0 {
		ttl = 30 * time.Second
	}
	if ttl < 10*time.Second {
		ttl = 10 * time.Second
	}
	return &StorageLeaderElection{Storage: store, Key: key, TTL: ttl}
}

type StorageLeaderElection struct {
	Storage store.Store
	Key     string
	TTL     time.Duration
}

func (le *StorageLeaderElection) OnLeader(ctx context.Context, onLeaderElected OnLeaderElected) error {
	lock := &StorageLeaderElectionLock{
		LeaseDuration:   le.TTL,
		RetryPeriod:     le.TTL / 2,
		RenewDeadline:   10 * time.Second,
		OnLeaderElected: onLeaderElected,
		ReleaseOnCancel: true,
		Name:            le.Key,
		Storage:         le.Storage,
	}
	if err := lock.run(ctx); err != nil {
		return fmt.Errorf("leader election failed: %w", err)
	}
	return nil
}

type StorageLeaderElectionLock struct {
	LeaseDuration time.Duration
	// RenewDeadline is the duration that the acting master will retry
	// refreshing leadership before giving up.
	//
	// Core clients default this value to 10 seconds.
	RenewDeadline time.Duration
	// RetryPeriod is the duration the LeaderElector clients should wait
	// between tries of actions.
	//
	// Core clients default this value to 2 seconds.
	RetryPeriod     time.Duration
	OnLeaderElected OnLeaderElected
	ReleaseOnCancel bool
	// Name is the name of the leader election lock.
	Name string
	// Identity is the identification of every client.
	Identity string
	Storage  store.Store

	observedTime       time.Time
	observedRecord     Lease
	observedRecordLock sync.Mutex
}

const JitterFactor = 1.2

func (le *StorageLeaderElectionLock) run(ctx context.Context) error {
	if !le.acquire(ctx) {
		return fmt.Errorf("failed to acquire lease %v", le.Name)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		if err := le.OnLeaderElected(ctx); err != nil {
			log.FromContext(ctx).Error(err, "OnLeaderElected failed")
			cancel()
		}
	}()
	return le.renew(ctx)
}

func (le *StorageLeaderElectionLock) acquire(ctx context.Context) bool {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	succeeded := false
	desc := le.Name
	klog.Infof("attempting to acquire leader lease %v...", desc)
	wait.JitterUntil(func() {
		succeeded = le.tryAcquireOrRenew(ctx)
		if !succeeded {
			klog.V(4).Infof("failed to acquire lease %v", desc)
			return
		}
		klog.Infof("successfully acquired lease %v", desc)
		cancel()
	}, le.RetryPeriod, JitterFactor, true, ctx.Done())
	return succeeded
}

func (le *StorageLeaderElectionLock) release() bool {
	if !le.IsLeader() {
		return true
	}
	now := meta.Now()
	leaderElectionRecord := &Lease{
		ObjectMeta:           store.ObjectMeta{Name: le.Name},
		LeaderTransitions:    le.observedRecord.LeaderTransitions,
		LeaseDurationSeconds: 1,
		RenewTime:            now,
		AcquireTime:          now,
	}
	if err := le.Storage.Update(context.TODO(), leaderElectionRecord); err != nil {
		klog.Errorf("Failed to release lock: %v", err)
		return false
	}
	le.setObservedRecord(leaderElectionRecord)
	return true
}

func (le *StorageLeaderElectionLock) renew(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	wait.Until(func() {
		timeoutCtx, timeoutCancel := context.WithTimeout(ctx, le.RenewDeadline)
		defer timeoutCancel()
		err := wait.PollUntilContextCancel(timeoutCtx, le.RetryPeriod, true, func(ctx context.Context) (bool, error) {
			return le.tryAcquireOrRenew(ctx), nil
		})
		desc := le.Name
		if err == nil {
			return
		}
		klog.Infof("failed to renew lease %v: %v", desc, err)
		cancel()
	}, le.RetryPeriod, ctx.Done())
	// if we hold the lease, give it up
	if le.ReleaseOnCancel {
		le.release()
	}
	return context.Canceled
}

func (l *StorageLeaderElectionLock) identity() string {
	if l.Identity == "" {
		l.Identity, _ = os.Hostname()
	}
	return l.Identity
}

func (le *StorageLeaderElectionLock) tryAcquireOrRenew(ctx context.Context) bool {
	now := meta.Now()
	leaderElectionRecord := &Lease{
		ObjectMeta:           store.ObjectMeta{Name: le.Name},
		HolderIdentity:       le.identity(),
		LeaseDurationSeconds: int(le.LeaseDuration / time.Second),
		RenewTime:            now,
		AcquireTime:          now,
	}
	// 1. fast path for the leader to update optimistically assuming that the record observed
	// last time is the current version.
	if le.IsLeader() && le.isLeaseValid(now) {
		oldObservedRecord := le.getObservedRecord()
		leaderElectionRecord.AcquireTime = oldObservedRecord.AcquireTime
		leaderElectionRecord.LeaderTransitions = oldObservedRecord.LeaderTransitions
		err := le.Storage.Update(ctx, leaderElectionRecord)
		if err == nil {
			le.setObservedRecord(leaderElectionRecord)
			return true
		}
		klog.Errorf("Failed to update lock optimitically: %v, falling back to slow path", err)
	}
	// 2. obtain or create the ElectionRecord
	oldLeaderElectionRecord := &Lease{}
	if err := le.Storage.Get(ctx, le.Name, oldLeaderElectionRecord); err != nil {
		if !errors.IsNotFound(err) {
			klog.Errorf("error retrieving resource lock %v: %v", le.Name, err)
			return false
		}
		if err = le.Storage.Create(ctx, leaderElectionRecord); err != nil {
			klog.Errorf("error initially creating leader election record: %v", err)
			return false
		}
		le.setObservedRecord(leaderElectionRecord)
		return true
	}

	// 3. Record obtained, check the Identity & Time
	// only use the expire time to check if the lease is valid
	// in case of the clock is not monotonic
	if le.getObservedRecord().ResourceVersion != oldLeaderElectionRecord.ResourceVersion {
		le.setObservedRecord(oldLeaderElectionRecord)
	}
	if len(oldLeaderElectionRecord.HolderIdentity) > 0 && le.isLeaseValid(now) && !le.IsLeader() {
		klog.V(4).Infof("lock is held by %v and has not yet expired", oldLeaderElectionRecord.HolderIdentity)
		return false
	}

	// 4. We're going to try to update. The leaderElectionRecord is set to it's default
	// here. Let's correct it before updating.
	if le.IsLeader() {
		leaderElectionRecord.AcquireTime = oldLeaderElectionRecord.AcquireTime
		leaderElectionRecord.LeaderTransitions = oldLeaderElectionRecord.LeaderTransitions
	} else {
		leaderElectionRecord.LeaderTransitions = oldLeaderElectionRecord.LeaderTransitions + 1
	}

	// update the lock itself
	if err := le.Storage.Update(ctx, leaderElectionRecord); err != nil {
		klog.Errorf("Failed to update lock: %v", err)
		return false
	}
	le.setObservedRecord(leaderElectionRecord)
	return true
}

// IsLeader returns true if the last observed leader was this client else returns false.
func (le *StorageLeaderElectionLock) IsLeader() bool {
	return le.getObservedRecord().HolderIdentity == le.identity()
}

func (le *StorageLeaderElectionLock) isLeaseValid(now meta.Time) bool {
	return le.observedTime.Add(time.Second * time.Duration(le.getObservedRecord().LeaseDurationSeconds)).After(now.Time)
}

func (le *StorageLeaderElectionLock) getObservedRecord() Lease {
	le.observedRecordLock.Lock()
	defer le.observedRecordLock.Unlock()
	return le.observedRecord
}

func (le *StorageLeaderElectionLock) setObservedRecord(observedRecord *Lease) {
	le.observedRecordLock.Lock()
	defer le.observedRecordLock.Unlock()

	le.observedRecord = *observedRecord
	le.observedTime = time.Now()
}
