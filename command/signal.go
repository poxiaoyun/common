package command

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// SignalContext returns a child context canceled by the first SIGINT or
// SIGTERM. A second signal exits the process immediately with status 1. Stop
// releases the signal registration and cancels the context.
func SignalContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	notifications := make(chan os.Signal, 2)
	stopped := make(chan struct{})
	signal.Notify(notifications, os.Interrupt, syscall.SIGTERM)
	go handleSignals(notifications, stopped, cancel, os.Exit)

	var once sync.Once
	return ctx, func() {
		once.Do(func() {
			signal.Stop(notifications)
			cancel()
			close(stopped)
		})
	}
}

func handleSignals(
	notifications <-chan os.Signal,
	stopped <-chan struct{},
	cancel context.CancelFunc,
	exit func(int),
) {
	select {
	case <-notifications:
		cancel()
	case <-stopped:
		return
	}
	select {
	case <-notifications:
		exit(1)
	case <-stopped:
	}
}
