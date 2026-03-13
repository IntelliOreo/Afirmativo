package interview

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/afirmativo/backend/internal/session"
	"github.com/afirmativo/backend/internal/shared"
)

// SubmitAnswerAsync queues one async answer job and starts background processing.
func (s *Service) SubmitAnswerAsync(ctx context.Context, sessionCode, answerText, questionText, turnID, clientRequestID string) (*SubmitAnswerAsyncResult, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, s.dbTimeout)
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
	if requestID := shared.RequestIDFromContext(ctx); requestID != "" {
		s.asyncAnswerRequestIDs.Store(job.ID, requestID)
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
		"request_id", s.asyncAnswerRequestID(job.ID),
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
	dbCtx, dbCancel := context.WithTimeout(ctx, s.dbTimeout)
	defer dbCancel()

	job, err := s.jobStore.GetAnswerJob(dbCtx, sessionCode, jobID)
	if err != nil {
		return nil, err
	}

	slog.Debug("async answer job fetched",
		"request_id", s.asyncAnswerRequestID(job.ID),
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

	payload, err := decodeAnswerJobPayload(job.ResultPayload)
	if err != nil {
		return nil, err
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

func (s *Service) asyncAnswerRequestID(jobID string) string {
	value, ok := s.asyncAnswerRequestIDs.Load(strings.TrimSpace(jobID))
	if !ok {
		return ""
	}
	requestID, _ := value.(string)
	return strings.TrimSpace(requestID)
}
