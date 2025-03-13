package controller

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/retry"
	"xiaoshiai.cn/common/store"
)

type Runable interface {
	Name() string
	Run(ctx context.Context) error
}

func NewControllerManager() *ControllerManager {
	return &ControllerManager{
		Controllers: map[string]Runable{},
	}
}

type ControllerManagerOptions struct {
	LeaderElection    bool   `json:"leaderElection"`
	LeaderElectionKey string `json:"leaderElectionKey"`
}

func (c *ControllerManager) WithStoreLeaderElection(storage store.Store, key string) *ControllerManager {
	storage = storage.Scope(store.Scope{Resource: "namespaces", Name: "leader-election"})
	c.LedaerElection = NewStoreLeaderElection(storage, key)
	return c
}

type ControllerManager struct {
	Controllers    map[string]Runable
	LedaerElection LeaderElection
}

func (c *ControllerManager) AddController(controller Runable) error {
	if c.Controllers == nil {
		c.Controllers = map[string]Runable{}
	}
	name := controller.Name()
	if _, ok := c.Controllers[name]; ok {
		return fmt.Errorf("controller %s already exists", name)
	}
	c.Controllers[name] = controller
	return nil
}

func (c *ControllerManager) Run(ctx context.Context) error {
	log := log.FromContext(ctx)
	if c.LedaerElection != nil {
		log.Info("controller manager run with leader election")
		return retry.Fixed(ctx, 10*time.Second, func(ctx context.Context) error {
			return c.LedaerElection.OnLeader(ctx, 0, func(ctx context.Context) error {
				log.Info("controller manager run on leader elected")
				return c.run(ctx)
			})
		})
	} else {
		log.Info("controller manager run without leader election")
		return c.run(ctx)
	}
}

func (c *ControllerManager) run(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)
	for name, controller := range c.Controllers {
		controller := controller
		eg.Go(func() error {
			if err := controller.Run(ctx); err != nil {
				log.FromContext(ctx).Error(err, "controller run failed", "controller", name)
				return err
			}
			return nil
		})
	}
	return eg.Wait()
}
