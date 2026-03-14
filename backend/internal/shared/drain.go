package shared

import (
	"context"
	"log/slog"
	"sync"
)

// WaitForWorkers blocks until all workers tracked by wg have exited or the
// context deadline is reached. The label is used in log messages to identify
// which worker pool is draining.
func WaitForWorkers(ctx context.Context, wg *sync.WaitGroup, label string) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		slog.Info(label + " workers drained")
	case <-ctx.Done():
		slog.Warn(label + " worker drain timed out")
	}
}
