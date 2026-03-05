// HTTP handlers for interview endpoints:
//
//	POST /api/interview/start  — HandleStart
package interview

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/afirmativo/backend/internal/session"
	"github.com/afirmativo/backend/internal/shared"
)

const maxJSONBody = 64 * 1024

// Handler holds interview HTTP handlers.
type Handler struct {
	svc                     *Service
	allowSensitiveDebugLogs bool
}

// NewHandler creates a Handler with the given service.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// SetAllowSensitiveDebugLogs controls whether debug payload logs may include sensitive fields.
// This should only be enabled for short-lived, controlled troubleshooting sessions.
func (h *Handler) SetAllowSensitiveDebugLogs(enabled bool) {
	h.allowSensitiveDebugLogs = enabled
}

type startRequest struct {
	SessionCode string `json:"sessionCode"`
	Language    string `json:"language"`
}

type questionResponse struct {
	TextEs         string `json:"textEs"`
	TextEn         string `json:"textEn"`
	Area           string `json:"area"`
	Kind           string `json:"kind"`
	TurnID         string `json:"turnId"`
	QuestionNumber int    `json:"questionNumber"`
	TotalQuestions int    `json:"totalQuestions"`
}

type startResponse struct {
	Question        questionResponse `json:"question"`
	TimerRemainingS int              `json:"timerRemainingS"`
	Resuming        bool             `json:"resuming"`
	Language        string           `json:"language"`
}

// HandleStart transitions a session to interviewing and returns the first question.
func (h *Handler) HandleStart(w http.ResponseWriter, r *http.Request) {
	var req startRequest
	if err := shared.DecodeJSON(r, &req, maxJSONBody); err != nil {
		shared.WriteError(w, shared.ErrBadRequest, "Invalid request body", "BAD_REQUEST")
		return
	}

	if req.SessionCode == "" {
		shared.WriteError(w, shared.ErrBadRequest, "Session code is required", "BAD_REQUEST")
		return
	}

	language, ok := normalizeRequestLanguage(req.Language)
	if !ok {
		shared.WriteError(w, shared.ErrBadRequest, "language must be es or en", "BAD_REQUEST")
		return
	}
	if !shared.RequireSessionCodeMatch(w, r, req.SessionCode) {
		return
	}

	slog.Debug("interview/start request",
		"session_code", req.SessionCode,
		"language", language,
	)

	result, err := h.svc.StartInterview(r.Context(), req.SessionCode, language)
	if err != nil {
		switch {
		case errors.Is(err, shared.ErrConflict):
			shared.WriteError(w, shared.ErrConflict, "Interview already completed", "INTERVIEW_COMPLETED")
		case errors.Is(err, shared.ErrNotFound):
			shared.WriteError(w, shared.ErrNotFound, "Session not found", "SESSION_NOT_FOUND")
		case errors.Is(err, session.ErrSessionExpired):
			shared.WriteError(w, shared.ErrGone, "Session expired", "SESSION_EXPIRED")
		default:
			slog.Error("interview start failed", "error", err)
			shared.WriteError(w, shared.ErrInternal, "Internal server error", "INTERNAL_ERROR")
		}
		return
	}

	q := result.Question
	shared.WriteJSON(w, http.StatusOK, startResponse{
		Question: questionResponse{
			TextEs:         q.TextEs,
			TextEn:         q.TextEn,
			Area:           q.Area,
			Kind:           string(q.Kind),
			TurnID:         q.TurnID,
			QuestionNumber: q.QuestionNumber,
			TotalQuestions: q.TotalQuestions,
		},
		TimerRemainingS: result.TimerRemainingS,
		Resuming:        result.Resuming,
		Language:        result.Language,
	})
}

const maxAnswerBody = 10 * 1024 // 10KB per spec

type answerAsyncRequest struct {
	SessionCode     string `json:"sessionCode"`
	AnswerText      string `json:"answerText"`
	QuestionText    string `json:"questionText"` // echoed back from frontend
	TurnID          string `json:"turnId"`
	ClientRequestID string `json:"clientRequestId"`
}

type answerAsyncAcceptedResponse struct {
	JobID           string `json:"jobId"`
	ClientRequestID string `json:"clientRequestId"`
	Status          string `json:"status"`
}

type answerJobStatusResponse struct {
	JobID           string            `json:"jobId"`
	ClientRequestID string            `json:"clientRequestId"`
	Status          string            `json:"status"`
	Done            bool              `json:"done"`
	NextQuestion    *questionResponse `json:"nextQuestion"`
	TimerRemainingS int               `json:"timerRemainingS"`
	ErrorCode       string            `json:"errorCode,omitempty"`
	ErrorMessage    string            `json:"errorMessage,omitempty"`
}

func normalizeRequestLanguage(language string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "", "es":
		return "es", true
	case "en":
		return "en", true
	default:
		return "", false
	}
}

func isSensitiveLogKey(key string) bool {
	lowerKey := strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(lowerKey, "pin") ||
		strings.Contains(lowerKey, "token") ||
		strings.Contains(lowerKey, "authorization") ||
		strings.Contains(lowerKey, "cookie") ||
		strings.Contains(lowerKey, "secret") ||
		strings.Contains(lowerKey, "password") ||
		strings.Contains(lowerKey, "api_key") ||
		strings.Contains(lowerKey, "apikey")
}

func redactSensitiveLogValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if isSensitiveLogKey(key) {
				out[key] = "[REDACTED]"
				continue
			}
			out[key] = redactSensitiveLogValue(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = redactSensitiveLogValue(item)
		}
		return out
	default:
		return value
	}
}

func (h *Handler) debugLogPayload(value any) any {
	if h.allowSensitiveDebugLogs && slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		return value
	}
	return redactSensitiveLogValue(value)
}

// HandleAnswerAsync accepts a text answer and queues async interview processing.
func (h *Handler) HandleAnswerAsync(w http.ResponseWriter, r *http.Request) {
	var req answerAsyncRequest
	if err := shared.DecodeJSON(r, &req, maxAnswerBody); err != nil {
		shared.WriteError(w, shared.ErrBadRequest, "Invalid request body", "BAD_REQUEST")
		return
	}

	if strings.TrimSpace(req.SessionCode) == "" {
		shared.WriteError(w, shared.ErrBadRequest, "sessionCode is required", "BAD_REQUEST")
		return
	}
	if strings.TrimSpace(req.TurnID) == "" {
		shared.WriteError(w, shared.ErrBadRequest, "turnId is required", "BAD_REQUEST")
		return
	}
	if strings.TrimSpace(req.ClientRequestID) == "" {
		shared.WriteError(w, shared.ErrBadRequest, "clientRequestId is required", "BAD_REQUEST")
		return
	}
	if !shared.RequireSessionCodeMatch(w, r, req.SessionCode) {
		return
	}

	// Match sync-style payload logging for local debugging at DEBUG level.
	debugPayload := map[string]any{
		"sessionCode":     strings.TrimSpace(req.SessionCode),
		"answerText":      req.AnswerText,
		"questionText":    req.QuestionText,
		"turnId":          strings.TrimSpace(req.TurnID),
		"clientRequestId": strings.TrimSpace(req.ClientRequestID),
	}
	slog.Debug("interview/answer-async request",
		"payload", h.debugLogPayload(debugPayload),
		"sensitive_debug_logs_enabled", h.allowSensitiveDebugLogs,
	)

	result, err := h.svc.SubmitAnswerAsync(
		r.Context(),
		req.SessionCode,
		req.AnswerText,
		req.QuestionText,
		req.TurnID,
		req.ClientRequestID,
	)
	if err != nil {
		switch {
		case errors.Is(err, ErrIdempotencyConflict):
			slog.Warn("answer async idempotency conflict",
				"session_code", req.SessionCode,
				"client_request_id", req.ClientRequestID,
				"turn_id", req.TurnID,
			)
			shared.WriteError(w, shared.ErrConflict, "clientRequestId cannot be reused with a different payload", "IDEMPOTENCY_CONFLICT")
		case errors.Is(err, session.ErrSessionExpired):
			shared.WriteError(w, shared.ErrGone, "Session expired", "SESSION_EXPIRED")
		default:
			slog.Error("answer async submission failed",
				"session_code", req.SessionCode,
				"client_request_id", req.ClientRequestID,
				"turn_id", req.TurnID,
				"error", err,
			)
			shared.WriteError(w, shared.ErrInternal, "Internal server error", "INTERNAL_ERROR")
		}
		return
	}

	slog.Info("answer async accepted",
		"session_code", req.SessionCode,
		"client_request_id", result.ClientRequestID,
		"job_id", result.JobID,
		"status", result.Status,
	)

	shared.WriteJSON(w, http.StatusAccepted, answerAsyncAcceptedResponse{
		JobID:           result.JobID,
		ClientRequestID: result.ClientRequestID,
		Status:          string(result.Status),
	})
}

// HandleAnswerJobStatus returns async answer job status and, once available, the computed result.
func (h *Handler) HandleAnswerJobStatus(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimSpace(r.PathValue("jobId"))
	sessionCode := strings.TrimSpace(r.URL.Query().Get("sessionCode"))
	if jobID == "" {
		shared.WriteError(w, shared.ErrBadRequest, "jobId is required", "BAD_REQUEST")
		return
	}
	if sessionCode == "" {
		shared.WriteError(w, shared.ErrBadRequest, "sessionCode is required", "BAD_REQUEST")
		return
	}
	if !shared.RequireSessionCodeMatch(w, r, sessionCode) {
		return
	}

	slog.Debug("interview/answer-job-status request",
		"session_code", sessionCode,
		"job_id", jobID,
	)

	result, err := h.svc.GetAnswerJobResult(r.Context(), sessionCode, jobID)
	if err != nil {
		switch {
		case errors.Is(err, ErrAsyncJobNotFound):
			slog.Warn("answer job status lookup not found",
				"session_code", sessionCode,
				"job_id", jobID,
			)
			shared.WriteError(w, shared.ErrNotFound, "Async answer job not found", "ASYNC_JOB_NOT_FOUND")
		default:
			slog.Error("answer job status lookup failed",
				"session_code", sessionCode,
				"job_id", jobID,
				"error", err,
			)
			shared.WriteError(w, shared.ErrInternal, "Internal server error", "INTERNAL_ERROR")
		}
		return
	}

	switch result.Status {
	case AsyncAnswerJobSucceeded, AsyncAnswerJobFailed, AsyncAnswerJobConflict, AsyncAnswerJobCanceled:
		slog.Info("answer job terminal status",
			"session_code", sessionCode,
			"job_id", result.JobID,
			"client_request_id", result.ClientRequestID,
			"status", result.Status,
			"done", result.Done,
			"error_code", result.ErrorCode,
		)
	default:
		slog.Debug("answer job in-progress status",
			"session_code", sessionCode,
			"job_id", result.JobID,
			"client_request_id", result.ClientRequestID,
			"status", result.Status,
		)
	}

	var nextQuestion *questionResponse
	if result.NextQuestion != nil {
		nextQuestion = &questionResponse{
			TextEs:         result.NextQuestion.TextEs,
			TextEn:         result.NextQuestion.TextEn,
			Area:           result.NextQuestion.Area,
			Kind:           string(result.NextQuestion.Kind),
			TurnID:         result.NextQuestion.TurnID,
			QuestionNumber: result.NextQuestion.QuestionNumber,
			TotalQuestions: result.NextQuestion.TotalQuestions,
		}
	}

	shared.WriteJSON(w, http.StatusOK, answerJobStatusResponse{
		JobID:           result.JobID,
		ClientRequestID: result.ClientRequestID,
		Status:          string(result.Status),
		Done:            result.Done,
		NextQuestion:    nextQuestion,
		TimerRemainingS: result.TimerRemainingS,
		ErrorCode:       result.ErrorCode,
		ErrorMessage:    result.ErrorMessage,
	})
}
