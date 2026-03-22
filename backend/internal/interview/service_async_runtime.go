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

const asyncAnswerIdleClaimEvery = 2 * time.Second

// StartAsyncAnswerRuntime launches bounded workers and periodic recovery.
func (s *Service) StartAsyncAnswerRuntime(ctx context.Context) {
	s.asyncRuntimeStartOnce.Do(func() {
		slog.Info("starting async answer runtime",
			"workers", s.asyncAnswerWorkers,
			"queue_size", cap(s.asyncAnswerQueue),
			"idle_claim_every", asyncAnswerIdleClaimEvery,
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
	idleTicker := time.NewTicker(asyncAnswerIdleClaimEvery)
	defer idleTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case jobID := <-s.asyncAnswerQueue:
			claimed, ok := s.claimAsyncAnswerJob(ctx, jobID, asyncAnswerClaimSourceChannel)
			if !ok {
				continue
			}
			s.processClaimedAsyncAnswerJobWithTrace(ctx, tracer, workerID, claimed)
		case <-idleTicker.C:
			claimed, ok := s.claimNextAsyncAnswerJob(ctx)
			if !ok {
				continue
			}
			s.processClaimedAsyncAnswerJobWithTrace(ctx, tracer, workerID, claimed)
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

	if requeued > 0 {
		slog.Info("async answer recovery cycle completed",
			"requeued_stale_running_jobs", requeued,
		)
	}
}

func (s *Service) enqueueAsyncAnswerJob(jobID, requestID string) bool {
	id := strings.TrimSpace(jobID)
	if id == "" {
		return false
	}
	if s.asyncAnswerQueue == nil {
		slog.Warn("async answer queue is not configured; job remains queued", "job_id", id, "request_id", requestID)
		return false
	}

	select {
	case s.asyncAnswerQueue <- id:
		slog.Debug("async answer job queued for worker pickup", "job_id", id, "request_id", requestID)
		return true
	default:
		// Leave status as queued in DB; idle DB fallback will pick it up later.
		slog.Warn("async answer queue is full; job remains queued", "job_id", id, "request_id", requestID)
		return false
	}
}

func (s *Service) processClaimedAsyncAnswerJobWithTrace(
	ctx context.Context,
	tracer trace.Tracer,
	workerID int,
	claimed *claimedAsyncAnswerJob,
) {
	if claimed == nil || ctx.Err() != nil {
		return
	}

	processCtx, cancel := context.WithTimeout(ctx, s.asyncAnswerJobTimeout)
	defer cancel()

	processCtx, span := tracer.Start(processCtx, "async.answer_job", trace.WithNewRoot())
	defer span.End()
	span.SetAttributes(
		attribute.String("job.id", claimed.job.ID),
		attribute.Int("worker.id", workerID),
		attribute.String("claim.source", claimed.claimSource),
	)

	s.processClaimedAsyncAnswerJob(processCtx, claimed)
}
