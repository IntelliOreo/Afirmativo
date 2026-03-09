package interview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/afirmativo/backend/internal/session"
)

const (
	defaultAsyncAnswerJobTimeout      = 3 * time.Minute
	defaultAsyncAnswerWorkers         = 4
	defaultAsyncAnswerQueueSize       = 256
	defaultAsyncAnswerRecoveryBatch   = 100
	defaultAsyncAnswerRecoveryEvery   = 10 * time.Second
	defaultAsyncAnswerStaleRunningAge = 3 * time.Minute
)

type AsyncConfig struct {
	Workers       int
	QueueSize     int
	RecoveryBatch int
	RecoveryEvery time.Duration
	StaleAfter    time.Duration
	JobTimeout    time.Duration
}

func (c AsyncConfig) withDefaults() AsyncConfig {
	if c.Workers <= 0 {
		c.Workers = defaultAsyncAnswerWorkers
	}
	if c.QueueSize <= 0 {
		c.QueueSize = defaultAsyncAnswerQueueSize
	}
	if c.RecoveryBatch <= 0 {
		c.RecoveryBatch = defaultAsyncAnswerRecoveryBatch
	}
	if c.RecoveryEvery <= 0 {
		c.RecoveryEvery = defaultAsyncAnswerRecoveryEvery
	}
	if c.StaleAfter <= 0 {
		c.StaleAfter = defaultAsyncAnswerStaleRunningAge
	}
	if c.JobTimeout <= 0 {
		c.JobTimeout = defaultAsyncAnswerJobTimeout
	}
	return c
}

// StartAsyncAnswerRuntime launches bounded workers and periodic recovery.
func (s *Service) StartAsyncAnswerRuntime(ctx context.Context) {
	s.asyncRuntimeStartOnce.Do(func() {
		if s.asyncAnswerQueue == nil {
			s.asyncAnswerQueue = make(chan string, defaultAsyncAnswerQueueSize)
		}

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
			go s.runAsyncAnswerWorker(ctx, workerID)
		}
		go s.runAsyncAnswerRecoveryLoop(ctx)
	})
}

func (s *Service) runAsyncAnswerWorker(ctx context.Context, workerID int) {
	slog.Debug("async answer worker started", "worker_id", workerID)
	defer slog.Debug("async answer worker stopped", "worker_id", workerID)

	for {
		select {
		case <-ctx.Done():
			return
		case jobID := <-s.asyncAnswerQueue:
			processCtx, cancel := context.WithTimeout(ctx, s.asyncAnswerJobTimeout)
			s.processAnswerJob(processCtx, jobID)
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
	requeueCtx, requeueCancel := context.WithTimeout(ctx, dbTimeout)
	requeued, err := s.jobStore.RequeueStaleRunningAnswerJobs(requeueCtx, staleBefore)
	requeueCancel()
	if err != nil {
		slog.Error("failed to requeue stale running async answer jobs", "error", err)
		return
	}

	listCtx, listCancel := context.WithTimeout(ctx, dbTimeout)
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
	if s.asyncAnswerQueue == nil {
		slog.Warn("async answer queue is not configured; job remains queued", "job_id", id)
		return false
	}

	select {
	case s.asyncAnswerQueue <- id:
		slog.Debug("async answer job queued for worker pickup", "job_id", id)
		return true
	default:
		// Leave status as queued in DB; recovery loop will retry enqueue.
		slog.Warn("async answer queue is full; job remains queued", "job_id", id)
		return false
	}
}

// SubmitAnswerAsyncResult is returned when an async answer job is accepted.
type SubmitAnswerAsyncResult struct {
	JobID           string
	ClientRequestID string
	Status          AsyncAnswerJobStatus
}

// AnswerJobStatusResult is returned by polling for async answer job state.
type AnswerJobStatusResult struct {
	JobID                        string
	ClientRequestID              string
	Status                       AsyncAnswerJobStatus
	Done                         bool
	NextQuestion                 *Question
	TimerRemainingS              int
	AnswerSubmitWindowRemainingS int
	ErrorCode                    string
	ErrorMessage                 string
}

type answerJobPayload struct {
	Done                         bool                    `json:"done"`
	NextQuestion                 *answerJobQuestionShape `json:"next_question"`
	TimerRemainingS              int                     `json:"timer_remaining_s"`
	AnswerSubmitWindowRemainingS int                     `json:"answer_submit_window_remaining_s"`
}

type answerJobQuestionShape struct {
	TextEs         string `json:"text_es"`
	TextEn         string `json:"text_en"`
	Area           string `json:"area"`
	Kind           string `json:"kind"`
	TurnID         string `json:"turn_id"`
	QuestionNumber int    `json:"question_number"`
	TotalQuestions int    `json:"total_questions"`
}

// SubmitAnswerAsync queues one async answer job and starts background processing.
func (s *Service) SubmitAnswerAsync(ctx context.Context, sessionCode, answerText, questionText, turnID, clientRequestID string) (*SubmitAnswerAsyncResult, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	sess, err := s.sessionGetter.GetSessionByCode(dbCtx, sessionCode)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	if s.nowFn().After(sess.ExpiresAt) {
		slog.Debug("submit answer async rejected: session expired", "session_code", sessionCode)
		return nil, session.ErrSessionExpired
	}

	job, err := s.jobStore.UpsertAnswerJob(dbCtx, UpsertAnswerJobParams{
		SessionCode:     sessionCode,
		ClientRequestID: clientRequestID,
		TurnID:          turnID,
		QuestionText:    questionText,
		AnswerText:      answerText,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert async answer job: %w", err)
	}

	// Idempotency key must map to the same semantic request payload.
	if strings.TrimSpace(job.TurnID) != strings.TrimSpace(turnID) ||
		strings.TrimSpace(job.AnswerText) != strings.TrimSpace(answerText) ||
		strings.TrimSpace(job.QuestionText) != strings.TrimSpace(questionText) {
		slog.Warn("async answer idempotency conflict",
			"session_code", sessionCode,
			"client_request_id", clientRequestID,
			"provided_turn_id", turnID,
			"stored_turn_id", job.TurnID,
		)
		return nil, ErrIdempotencyConflict
	}

	triggered := false
	if job.Status == AsyncAnswerJobQueued {
		triggered = s.enqueueAsyncAnswerJob(job.ID)
	}

	slog.Info("async answer job accepted",
		"session_code", job.SessionCode,
		"client_request_id", job.ClientRequestID,
		"job_id", job.ID,
		"status", job.Status,
		"attempts", job.Attempts,
		"worker_triggered", triggered,
	)

	return &SubmitAnswerAsyncResult{
		JobID:           job.ID,
		ClientRequestID: job.ClientRequestID,
		Status:          job.Status,
	}, nil
}

// GetAnswerJobResult returns current async job status and, when available, the computed next question payload.
func (s *Service) GetAnswerJobResult(ctx context.Context, sessionCode, jobID string) (*AnswerJobStatusResult, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	job, err := s.jobStore.GetAnswerJob(dbCtx, sessionCode, jobID)
	if err != nil {
		return nil, err
	}

	slog.Debug("async answer job fetched",
		"session_code", sessionCode,
		"job_id", job.ID,
		"client_request_id", job.ClientRequestID,
		"status", job.Status,
		"attempts", job.Attempts,
	)

	result := &AnswerJobStatusResult{
		JobID:           job.ID,
		ClientRequestID: job.ClientRequestID,
		Status:          job.Status,
		ErrorCode:       job.ErrorCode,
		ErrorMessage:    job.ErrorMessage,
	}

	if job.Status != AsyncAnswerJobSucceeded || len(job.ResultPayload) == 0 {
		return result, nil
	}

	var payload answerJobPayload
	if err := json.Unmarshal(job.ResultPayload, &payload); err != nil {
		return nil, fmt.Errorf("decode answer job payload: %w", err)
	}

	result.Done = payload.Done
	result.TimerRemainingS = payload.TimerRemainingS
	result.AnswerSubmitWindowRemainingS = payload.AnswerSubmitWindowRemainingS
	if payload.NextQuestion != nil {
		result.NextQuestion = &Question{
			TextEs:         payload.NextQuestion.TextEs,
			TextEn:         payload.NextQuestion.TextEn,
			Area:           payload.NextQuestion.Area,
			Kind:           QuestionKind(payload.NextQuestion.Kind),
			TurnID:         payload.NextQuestion.TurnID,
			QuestionNumber: payload.NextQuestion.QuestionNumber,
			TotalQuestions: payload.NextQuestion.TotalQuestions,
		}
	}

	return result, nil
}

func (s *Service) processAnswerJob(ctx context.Context, jobID string) {
	claimCtx, claimCancel := context.WithTimeout(ctx, dbTimeout)
	job, err := s.jobStore.ClaimQueuedAnswerJob(claimCtx, jobID)
	claimCancel()
	if err != nil {
		slog.Error("failed to claim async answer job", "job_id", jobID, "error", err)
		return
	}
	if job == nil {
		slog.Debug("async answer job not claimable", "job_id", jobID)
		return
	}

	slog.Info("async answer job claimed",
		"session_code", job.SessionCode,
		"client_request_id", job.ClientRequestID,
		"job_id", job.ID,
		"status", job.Status,
		"attempts", job.Attempts,
	)

	answerResult, err := s.processTurnForAsyncJob(ctx, job)
	if err != nil {
		status := AsyncAnswerJobFailed
		errorCode := "INTERNAL_ERROR"
		errorMessage := "Internal server error"
		if errors.Is(err, ErrTurnConflict) {
			status = AsyncAnswerJobConflict
			errorCode = "TURN_CONFLICT"
			errorMessage = "Turn is stale or out of order"
		} else if errors.Is(err, ErrInvalidFlow) {
			errorCode = "FLOW_INVALID"
			errorMessage = "Interview flow is not in a valid state"
		} else if errors.Is(err, ErrAIRetryExhausted) {
			status = AsyncAnswerJobCanceled
			errorCode = "AI_RETRY_EXHAUSTED"
			errorMessage = "AI processing was unstable after retries. Reload to continue."
		}

		slog.Warn("async answer job processing failed",
			"session_code", job.SessionCode,
			"client_request_id", job.ClientRequestID,
			"job_id", job.ID,
			"status", status,
			"error_code", errorCode,
			"error", err,
		)

		failCtx, failCancel := context.WithTimeout(ctx, dbTimeout)
		markErr := s.jobStore.MarkAnswerJobFailed(failCtx, MarkAnswerJobFailedParams{
			JobID:        job.ID,
			Status:       status,
			ErrorCode:    errorCode,
			ErrorMessage: errorMessage,
		})
		failCancel()
		if markErr != nil {
			slog.Error("failed to mark async answer job as failed",
				"session_code", job.SessionCode,
				"client_request_id", job.ClientRequestID,
				"job_id", job.ID,
				"status", status,
				"error_code", errorCode,
				"error", markErr,
			)
		} else {
			slog.Info("async answer job marked terminal",
				"session_code", job.SessionCode,
				"client_request_id", job.ClientRequestID,
				"job_id", job.ID,
				"status", status,
				"error_code", errorCode,
			)
		}
		return
	}

	if answerResult.Substituted {
		cancelCtx, cancelCancel := context.WithTimeout(ctx, dbTimeout)
		markErr := s.jobStore.MarkAnswerJobFailed(cancelCtx, MarkAnswerJobFailedParams{
			JobID:        job.ID,
			Status:       AsyncAnswerJobCanceled,
			ErrorCode:    "AI_RETRY_EXHAUSTED",
			ErrorMessage: "AI retries exhausted and fallback substitution was applied. Reload to continue.",
		})
		cancelCancel()
		if markErr != nil {
			slog.Error("failed to mark async answer job as canceled",
				"session_code", job.SessionCode,
				"client_request_id", job.ClientRequestID,
				"job_id", job.ID,
				"error", markErr,
			)
		} else {
			slog.Info("async answer job marked terminal",
				"session_code", job.SessionCode,
				"client_request_id", job.ClientRequestID,
				"job_id", job.ID,
				"status", AsyncAnswerJobCanceled,
				"error_code", "AI_RETRY_EXHAUSTED",
			)
		}
		return
	}

	payload, err := json.Marshal(toAnswerJobPayload(answerResult))
	if err != nil {
		failCtx, failCancel := context.WithTimeout(ctx, dbTimeout)
		markErr := s.jobStore.MarkAnswerJobFailed(failCtx, MarkAnswerJobFailedParams{
			JobID:        job.ID,
			Status:       AsyncAnswerJobFailed,
			ErrorCode:    "SERIALIZATION_ERROR",
			ErrorMessage: "Failed to serialize async job result",
		})
		failCancel()
		if markErr != nil {
			slog.Error("failed to mark async answer job serialization failure",
				"session_code", job.SessionCode,
				"client_request_id", job.ClientRequestID,
				"job_id", job.ID,
				"error", markErr,
			)
		} else {
			slog.Warn("async answer job serialization failure",
				"session_code", job.SessionCode,
				"client_request_id", job.ClientRequestID,
				"job_id", job.ID,
				"error_code", "SERIALIZATION_ERROR",
			)
		}
		return
	}

	successCtx, successCancel := context.WithTimeout(ctx, dbTimeout)
	if err := s.jobStore.MarkAnswerJobSucceeded(successCtx, job.ID, payload); err != nil {
		slog.Error("failed to mark async answer job as succeeded",
			"session_code", job.SessionCode,
			"client_request_id", job.ClientRequestID,
			"job_id", job.ID,
			"error", err,
		)
	} else {
		slog.Info("async answer job marked terminal",
			"session_code", job.SessionCode,
			"client_request_id", job.ClientRequestID,
			"job_id", job.ID,
			"status", AsyncAnswerJobSucceeded,
		)
	}
	successCancel()
}

func toAnswerJobPayload(result *AnswerResult) *answerJobPayload {
	payload := &answerJobPayload{
		Done:                         result.Done,
		TimerRemainingS:              result.TimerRemainingS,
		AnswerSubmitWindowRemainingS: result.AnswerSubmitWindowRemainingS,
	}
	if result.NextQuestion != nil {
		payload.NextQuestion = &answerJobQuestionShape{
			TextEs:         result.NextQuestion.TextEs,
			TextEn:         result.NextQuestion.TextEn,
			Area:           result.NextQuestion.Area,
			Kind:           string(result.NextQuestion.Kind),
			TurnID:         result.NextQuestion.TurnID,
			QuestionNumber: result.NextQuestion.QuestionNumber,
			TotalQuestions: result.NextQuestion.TotalQuestions,
		}
	}
	return payload
}

func (s *Service) newAsyncJobRetryFailureRecorder(jobID string) aiRetryFailureRecorder {
	trimmedJobID := strings.TrimSpace(jobID)
	if trimmedJobID == "" {
		return nil
	}

	return aiRetryFailureRecorderFunc(func(ctx context.Context, reason string, incrementAttempts bool) {
		appendCtx, appendCancel := context.WithTimeout(ctx, dbTimeout)
		appendErr := s.jobStore.AppendAnswerJobFailedReason(appendCtx, trimmedJobID, reason)
		appendCancel()
		if appendErr != nil {
			slog.Warn("failed to append async answer retry reason", "job_id", trimmedJobID, "error", appendErr)
		}

		if !incrementAttempts {
			return
		}

		incCtx, incCancel := context.WithTimeout(ctx, dbTimeout)
		incErr := s.jobStore.IncrementAnswerJobAttempts(incCtx, trimmedJobID)
		incCancel()
		if incErr != nil {
			slog.Warn("failed to increment async answer attempts for retry", "job_id", trimmedJobID, "error", incErr)
		}
	})
}
