package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// ContextWithStopChan creates a context canceled when the given stopCh receives a message
// or get closed.
func ContextWithStopChan(ctx context.Context, stopCh <-chan struct{}) context.Context {
	ctx, cancel := context.WithCancel(ctx)

	go func() {
		defer cancel()

		select {
		case <-ctx.Done():
		case <-stopCh:
		}
	}()

	return ctx
}

// ContextWithSignal creates a context canceled when SIGINT or SIGTERM are notified.
func ContextWithSignal(ctx context.Context) context.Context {
	newCtx, cancel := context.WithCancel(ctx)
	signals := make(chan os.Signal)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signals
		cancel()
	}()

	return newCtx
}
