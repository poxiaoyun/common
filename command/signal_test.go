package command

import (
	"context"
	"os"
	"syscall"
	"testing"
)

func TestHandleSignalsCancelsThenExits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	notifications := make(chan os.Signal, 2)
	notifications <- os.Interrupt
	notifications <- syscall.SIGTERM

	exitCode := 0
	canceledBeforeExit := false
	handleSignals(notifications, make(chan struct{}), cancel, func(code int) {
		exitCode = code
		canceledBeforeExit = ctx.Err() == context.Canceled
	})

	if ctx.Err() != context.Canceled {
		t.Fatal("first signal did not cancel the context")
	}
	if exitCode != 1 || !canceledBeforeExit {
		t.Fatalf("second signal exit = %d after cancellation %t", exitCode, canceledBeforeExit)
	}
}

func TestSignalContextStopIsReusable(t *testing.T) {
	ctx, stop := SignalContext(context.Background())
	stop()
	stop()
	if ctx.Err() != context.Canceled {
		t.Fatal("stop did not cancel the context")
	}
}
