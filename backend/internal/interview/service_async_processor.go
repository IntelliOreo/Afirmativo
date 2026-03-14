package interview

import (
	"context"
	"errors"
	"log/slog"
	"strings"
)

type claimedAsyncAnswerJob struct {
	job       *AnswerJob
	requestID string
}

type asyncAnswerTerminalOutcome struct {
	status       AsyncAnswerJobStatus
	errorCode    string
	errorMessage string
}

// maxAsyncAnswerJobAttempts is the ceiling before a job is marked as permanently failed.
const maxAsyncAnswerJobAttempts = 5

func (s *Service) processAnswerJob(ctx context.Context, jobID string) {
	claimed, ok := s.claimAsyncAnswerJob(ctx, jobID)
	if !ok {
		return
	}
	defer s.asyncAnswerRequestIDs.Delete(claimed.job.ID)

	// If the job has been retried too many times, mark it as permanently failed.
	if claimed.job.Attempts > maxAsyncAnswerJobAttempts {
		slog.Error("async answer job exceeded max attempts",
			"request_id", claimed.requestID,
			"job_id", claimed.job.ID,
			"session_code", claimed.job.SessionCode,
			"attempts", claimed.job.Attempts,
		)
		s.finalizeAsyncAnswerJobTerminal(ctx, claimed, asyncAnswerTerminalOutcome{
			status:       AsyncAnswerJobFailed,
			errorCode:    "SESSION_COMPLETE_FAILED",
			errorMessage: "Session completion failed after maximum retry attempts",
		}, ErrSessionCompleteFailed)
		return
	}

	answerResult, err := s.processTurnForAsyncJob(ctx, claimed.job)
	if err != nil {
		// Session completion failure: leave job in "running" so the recovery
		// loop requeues it and retries on the next cycle.
		if errors.Is(err, ErrSessionCompleteFailed) {
			slog.Warn("session completion failed; leaving job running for recovery retry",
				"request_id", claimed.requestID,
				"job_id", claimed.job.ID,
				"session_code", claimed.job.SessionCode,
				"attempts", claimed.job.Attempts,
				"error", err,
			)
			return
		}
		outcome := classifyAsyncAnswerTerminalOutcome(err)
		s.finalizeAsyncAnswerJobTerminal(ctx, claimed, outcome, err)
		return
	}

	if outcome, terminal := classifyAsyncAnswerCompletionOutcome(answerResult); terminal {
		s.finalizeAsyncAnswerJobTerminal(ctx, claimed, outcome, nil)
		return
	}

	s.finalizeAsyncAnswerJobSuccess(ctx, claimed, answerResult)
}

func (s *Service) claimAsyncAnswerJob(ctx context.Context, jobID string) (*claimedAsyncAnswerJob, bool) {
	claimCtx, claimCancel := context.WithTimeout(ctx, s.dbTimeout)
	job, err := s.jobStore.ClaimQueuedAnswerJob(claimCtx, jobID)
	claimCancel()
	requestID := s.asyncAnswerRequestID(jobID)
	if err != nil {
		slog.Error("failed to claim async answer job", "job_id", jobID, "request_id", requestID, "error", err)
		return nil, false
	}
	if job == nil {
		slog.Debug("async answer job not claimable", "job_id", jobID, "request_id", requestID)
		return nil, false
	}

	slog.Info("async answer job claimed",
		"request_id", requestID,
		"session_code", job.SessionCode,
		"client_request_id", job.ClientRequestID,
		"job_id", job.ID,
		"status", job.Status,
		"attempts", job.Attempts,
	)

	return &claimedAsyncAnswerJob{job: job, requestID: requestID}, true
}

func classifyAsyncAnswerTerminalOutcome(err error) asyncAnswerTerminalOutcome {
	outcome := asyncAnswerTerminalOutcome{
		status:       AsyncAnswerJobFailed,
		errorCode:    "INTERNAL_ERROR",
		errorMessage: "Internal server error",
	}
	switch {
	case errors.Is(err, ErrTurnConflict):
		outcome.status = AsyncAnswerJobConflict
		outcome.errorCode = "TURN_CONFLICT"
		outcome.errorMessage = "Turn is stale or out of order"
	case errors.Is(err, ErrInvalidFlow):
		outcome.errorCode = "FLOW_INVALID"
		outcome.errorMessage = "Interview flow is not in a valid state"
	case errors.Is(err, ErrAIRetryExhausted):
		outcome.status = AsyncAnswerJobCanceled
		outcome.errorCode = "AI_RETRY_EXHAUSTED"
		outcome.errorMessage = "AI processing was unstable after retries. Reload to continue."
	}
	return outcome
}

func substitutedAsyncAnswerOutcome() asyncAnswerTerminalOutcome {
	return asyncAnswerTerminalOutcome{
		status:       AsyncAnswerJobCanceled,
		errorCode:    "AI_RETRY_EXHAUSTED",
		errorMessage: "AI retries exhausted and fallback substitution was applied. Reload to continue.",
	}
}

func classifyAsyncAnswerCompletionOutcome(answerResult *AnswerResult) (asyncAnswerTerminalOutcome, bool) {
	if answerResult == nil || !answerResult.Substituted {
		return asyncAnswerTerminalOutcome{}, false
	}
	return substitutedAsyncAnswerOutcome(), true
}

func serializationAsyncAnswerOutcome() asyncAnswerTerminalOutcome {
	return asyncAnswerTerminalOutcome{
		status:       AsyncAnswerJobFailed,
		errorCode:    "SERIALIZATION_ERROR",
		errorMessage: "Failed to serialize async job result",
	}
}

func (s *Service) finalizeAsyncAnswerJobTerminal(
	ctx context.Context,
	claimed *claimedAsyncAnswerJob,
	outcome asyncAnswerTerminalOutcome,
	cause error,
) {
	if cause != nil {
		slog.Warn("async answer job terminalized without success",
			"request_id", claimed.requestID,
			"session_code", claimed.job.SessionCode,
			"client_request_id", claimed.job.ClientRequestID,
			"job_id", claimed.job.ID,
			"status", outcome.status,
			"error_code", outcome.errorCode,
			"error", cause,
		)
	}

	terminalCtx, terminalCancel := context.WithTimeout(ctx, s.dbTimeout)
	markErr := s.jobStore.MarkAnswerJobFailed(terminalCtx, MarkAnswerJobFailedParams{
		JobID:        claimed.job.ID,
		Status:       outcome.status,
		ErrorCode:    outcome.errorCode,
		ErrorMessage: outcome.errorMessage,
	})
	terminalCancel()
	if markErr != nil {
		slog.Error("failed to mark async answer job as terminal",
			"request_id", claimed.requestID,
			"session_code", claimed.job.SessionCode,
			"client_request_id", claimed.job.ClientRequestID,
			"job_id", claimed.job.ID,
			"status", outcome.status,
			"error_code", outcome.errorCode,
			"error", markErr,
		)
		return
	}

	slog.Info("async answer job marked terminal",
		"request_id", claimed.requestID,
		"session_code", claimed.job.SessionCode,
		"client_request_id", claimed.job.ClientRequestID,
		"job_id", claimed.job.ID,
		"status", outcome.status,
		"error_code", outcome.errorCode,
	)
}

func (s *Service) finalizeAsyncAnswerJobSuccess(ctx context.Context, claimed *claimedAsyncAnswerJob, answerResult *AnswerResult) {
	payload, err := encodeAnswerJobPayload(answerResult)
	if err != nil {
		s.finalizeAsyncAnswerJobTerminal(ctx, claimed, serializationAsyncAnswerOutcome(), err)
		return
	}

	successCtx, successCancel := context.WithTimeout(ctx, s.dbTimeout)
	if err := s.jobStore.MarkAnswerJobSucceeded(successCtx, claimed.job.ID, payload); err != nil {
		slog.Error("failed to mark async answer job as succeeded",
			"request_id", claimed.requestID,
			"session_code", claimed.job.SessionCode,
			"client_request_id", claimed.job.ClientRequestID,
			"job_id", claimed.job.ID,
			"error", err,
		)
		successCancel()
		return
	}
	successCancel()

	slog.Info("async answer job marked terminal",
		"request_id", claimed.requestID,
		"session_code", claimed.job.SessionCode,
		"client_request_id", claimed.job.ClientRequestID,
		"job_id", claimed.job.ID,
		"status", AsyncAnswerJobSucceeded,
	)
}

func (s *Service) newAsyncJobRetryFailureRecorder(jobID string) aiRetryFailureRecorder {
	trimmedJobID := strings.TrimSpace(jobID)
	if trimmedJobID == "" {
		return nil
	}

	return aiRetryFailureRecorderFunc(func(ctx context.Context, reason string, incrementAttempts bool) {
		appendCtx, appendCancel := context.WithTimeout(ctx, s.dbTimeout)
		appendErr := s.jobStore.AppendAnswerJobFailedReason(appendCtx, trimmedJobID, reason)
		appendCancel()
		if appendErr != nil {
			slog.Warn("failed to append async answer retry reason", "job_id", trimmedJobID, "error", appendErr)
		}

		if !incrementAttempts {
			return
		}

		incCtx, incCancel := context.WithTimeout(ctx, s.dbTimeout)
		incErr := s.jobStore.IncrementAnswerJobAttempts(incCtx, trimmedJobID)
		incCancel()
		if incErr != nil {
			slog.Warn("failed to increment async answer attempts for retry", "job_id", trimmedJobID, "error", incErr)
		}
	})
}
