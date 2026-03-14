package interview

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/afirmativo/backend/internal/shared"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// StartAsyncAnswerRuntime launches bounded workers and periodic recovery.
func (s *Service) StartAsyncAnswerRuntime(ctx context.Context) {
	s.asyncRuntimeStartOnce.Do(func() {
		slog.Info("starting async answer runtime",
			"workers", s.asyncAnswerWorkers,
			"queue_size", cap(s.asyncAnswerQueue),
			"recovery_batch", s.asyncAnswerRecoveryBatch,
			"recovery_every", s.asyncAnswerRecoveryEvery,
			"stale_after", s.asyncAnswerStaleAfter,
			"job_timeout", s.asyncAnswerJobTimeout,
		)

		for i := 0; i < s.asyncAnswerWorkers; i++ {
			workerID := i + 1
			s.workerWg.Add(1)
			go s.runAsyncAnswerWorker(ctx, workerID)
		}
		go s.runAsyncAnswerRecoveryLoop(ctx)
	})
}

// WaitForDrain blocks until all async answer workers have exited or the
// context deadline is reached. Call after cancelling the runtime context.
func (s *Service) WaitForDrain(ctx context.Context) {
	shared.WaitForWorkers(ctx, &s.workerWg, "async answer")
}

func (s *Service) runAsyncAnswerWorker(ctx context.Context, workerID int) {
	defer s.workerWg.Done()
	slog.Debug("async answer worker started", "worker_id", workerID)
	defer slog.Debug("async answer worker stopped", "worker_id", workerID)

	tracer := otel.Tracer("afirmativo-async")
	for {
		select {
		case <-ctx.Done():
			return
		case jobID := <-s.asyncAnswerQueue:
			processCtx, cancel := context.WithTimeout(ctx, s.asyncAnswerJobTimeout)
			processCtx, span := tracer.Start(processCtx, "async.answer_job",
				trace.WithNewRoot(),
			)
			span.SetAttributes(
				attribute.String("job.id", jobID),
				attribute.Int("worker.id", workerID),
			)
			s.processAnswerJob(processCtx, jobID)
			span.End()
			cancel()
		}
	}
}

func (s *Service) runAsyncAnswerRecoveryLoop(ctx context.Context) {
	s.recoverAsyncAnswerJobs(ctx)

	ticker := time.NewTicker(s.asyncAnswerRecoveryEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.recoverAsyncAnswerJobs(ctx)
		}
	}
}

func (s *Service) recoverAsyncAnswerJobs(ctx context.Context) {
	staleBefore := s.nowFn().UTC().Add(-s.asyncAnswerStaleAfter)
	requeueCtx, requeueCancel := context.WithTimeout(ctx, s.dbTimeout)
	requeued, err := s.jobStore.RequeueStaleRunningAnswerJobs(requeueCtx, staleBefore)
	requeueCancel()
	if err != nil {
		slog.Error("failed to requeue stale running async answer jobs", "error", err)
		return
	}

	listCtx, listCancel := context.WithTimeout(ctx, s.dbTimeout)
	queuedIDs, err := s.jobStore.ListQueuedAnswerJobIDs(listCtx, s.asyncAnswerRecoveryBatch)
	listCancel()
	if err != nil {
		slog.Error("failed to list queued async answer jobs", "error", err)
		return
	}

	enqueued := 0
	for _, jobID := range queuedIDs {
		if s.enqueueAsyncAnswerJob(jobID) {
			enqueued++
		}
	}

	if requeued > 0 || enqueued > 0 {
		slog.Info("async answer recovery cycle completed",
			"requeued_stale_running_jobs", requeued,
			"queued_jobs_listed", len(queuedIDs),
			"queued_jobs_enqueued", enqueued,
		)
	}
}

func (s *Service) enqueueAsyncAnswerJob(jobID string) bool {
	id := strings.TrimSpace(jobID)
	if id == "" {
		return false
	}
	requestID := s.asyncAnswerRequestID(id)
	if s.asyncAnswerQueue == nil {
		slog.Warn("async answer queue is not configured; job remains queued", "job_id", id, "request_id", requestID)
		return false
	}

	select {
	case s.asyncAnswerQueue <- id:
		slog.Debug("async answer job queued for worker pickup", "job_id", id, "request_id", requestID)
		return true
	default:
		// Leave status as queued in DB; recovery loop will retry enqueue.
		slog.Warn("async answer queue is full; job remains queued", "job_id", id, "request_id", requestID)
		return false
	}
}
