package orchkit

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// signalContext returns a context that is cancelled when SIGINT or SIGTERM
// is received. The stop function releases resources — always defer it.
func signalContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-ch:
			cancel()
		case <-ctx.Done():
		}
		signal.Stop(ch)
	}()
	return ctx, cancel
}
