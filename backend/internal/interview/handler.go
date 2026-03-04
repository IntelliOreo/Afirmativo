// HTTP handlers for interview endpoints:
//
//	POST /api/interview/start  — HandleStart
//	POST /api/interview/answer — HandleAnswer
package interview

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/afirmativo/backend/internal/shared"
)

const maxJSONBody = 64 * 1024

// Handler holds interview HTTP handlers.
type Handler struct {
	svc *Service
}

// NewHandler creates a Handler with the given service.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
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

type answerResponse struct {
	Done            bool              `json:"done"`
	NextQuestion    *questionResponse `json:"nextQuestion"`
	TimerRemainingS int               `json:"timerRemainingS"`
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

	slog.Debug("interview/start payload", "body", req)

	result, err := h.svc.StartInterview(r.Context(), req.SessionCode, language)
	if err != nil {
		switch {
		case errors.Is(err, shared.ErrConflict):
			shared.WriteError(w, shared.ErrConflict, "Interview already completed", "INTERVIEW_COMPLETED")
		case errors.Is(err, shared.ErrNotFound):
			shared.WriteError(w, shared.ErrNotFound, "Session not found", "SESSION_NOT_FOUND")
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

type answerRequest struct {
	SessionCode  string `json:"sessionCode"`
	AnswerText   string `json:"answerText"`
	QuestionText string `json:"questionText"` // echoed back from frontend
	TurnID       string `json:"turnId"`
}

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

// HandleAnswer accepts a text answer, calls the AI API for the next question, and returns it.
func (h *Handler) HandleAnswer(w http.ResponseWriter, r *http.Request) {
	var req answerRequest
	if err := shared.DecodeJSON(r, &req, maxAnswerBody); err != nil {
		shared.WriteError(w, shared.ErrBadRequest, "Invalid request body", "BAD_REQUEST")
		return
	}

	if req.SessionCode == "" {
		shared.WriteError(w, shared.ErrBadRequest, "sessionCode is required", "BAD_REQUEST")
		return
	}
	if strings.TrimSpace(req.TurnID) == "" {
		shared.WriteError(w, shared.ErrBadRequest, "turnId is required", "BAD_REQUEST")
		return
	}

	slog.Debug("interview/answer payload", "body", req)

	result, err := h.svc.SubmitAnswer(r.Context(), req.SessionCode, req.AnswerText, req.QuestionText, req.TurnID)
	if err != nil {
		switch {
		case errors.Is(err, ErrTurnConflict):
			shared.WriteError(w, shared.ErrConflict, "Turn is stale or out of order", "TURN_CONFLICT")
		case errors.Is(err, ErrInvalidFlow):
			shared.WriteError(w, shared.ErrConflict, "Interview flow is not in a valid state", "FLOW_INVALID")
		default:
			slog.Error("answer submission failed", "error", err)
			shared.WriteError(w, shared.ErrInternal, "Internal server error", "INTERNAL_ERROR")
		}
		return
	}

	if result.Done {
		shared.WriteJSON(w, http.StatusOK, answerResponse{Done: true, TimerRemainingS: result.TimerRemainingS})
		return
	}

	q := result.NextQuestion
	shared.WriteJSON(w, http.StatusOK, answerResponse{
		Done: false,
		NextQuestion: &questionResponse{
			TextEs:         q.TextEs,
			TextEn:         q.TextEn,
			Area:           q.Area,
			Kind:           string(q.Kind),
			TurnID:         q.TurnID,
			QuestionNumber: q.QuestionNumber,
			TotalQuestions: q.TotalQuestions,
		},
		TimerRemainingS: result.TimerRemainingS,
	})
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
			shared.WriteError(w, shared.ErrConflict, "clientRequestId cannot be reused with a different payload", "IDEMPOTENCY_CONFLICT")
		default:
			slog.Error("answer async submission failed", "error", err)
			shared.WriteError(w, shared.ErrInternal, "Internal server error", "INTERNAL_ERROR")
		}
		return
	}

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

	result, err := h.svc.GetAnswerJobResult(r.Context(), sessionCode, jobID)
	if err != nil {
		switch {
		case errors.Is(err, ErrAsyncJobNotFound):
			shared.WriteError(w, shared.ErrNotFound, "Async answer job not found", "ASYNC_JOB_NOT_FOUND")
		default:
			slog.Error("answer job status lookup failed", "error", err)
			shared.WriteError(w, shared.ErrInternal, "Internal server error", "INTERNAL_ERROR")
		}
		return
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
