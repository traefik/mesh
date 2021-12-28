package cmd

import (
	"context"
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
