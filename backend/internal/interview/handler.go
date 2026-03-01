// HTTP handlers for interview endpoints:
//
//	POST /api/interview/start  — HandleStart
//	POST /api/interview/answer — HandleAnswer
package interview

import (
	"errors"
	"log/slog"
	"net/http"

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
}

type questionResponse struct {
	TextEs         string `json:"textEs"`
	TextEn         string `json:"textEn"`
	Area           string `json:"area"`
	QuestionNumber int    `json:"questionNumber"`
	TotalQuestions int    `json:"totalQuestions"`
}

type startResponse struct {
	Question        questionResponse `json:"question"`
	TimerRemainingS int              `json:"timer_remaining_s"`
}

type answerResponse struct {
	Done         bool              `json:"done"`
	NextQuestion *questionResponse `json:"nextQuestion"`
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

	slog.Debug("interview/start", "session", req.SessionCode)

	result, err := h.svc.StartInterview(r.Context(), req.SessionCode)
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
			QuestionNumber: q.QuestionNumber,
			TotalQuestions: q.TotalQuestions,
		},
		TimerRemainingS: result.TimerRemainingS,
	})
}

const maxAnswerBody = 10 * 1024 // 10KB per spec

type answerRequest struct {
	SessionCode    string `json:"sessionCode"`
	AnswerText     string `json:"answerText"`
	QuestionNumber int    `json:"questionNumber"`
}

// HandleAnswer accepts a text answer, calls the AI API for the next question, and returns it.
func (h *Handler) HandleAnswer(w http.ResponseWriter, r *http.Request) {
	var req answerRequest
	if err := shared.DecodeJSON(r, &req, maxAnswerBody); err != nil {
		shared.WriteError(w, shared.ErrBadRequest, "Invalid request body", "BAD_REQUEST")
		return
	}

	if req.SessionCode == "" || req.AnswerText == "" {
		shared.WriteError(w, shared.ErrBadRequest, "sessionCode and answerText are required", "BAD_REQUEST")
		return
	}

	slog.Debug("interview/answer", "session", req.SessionCode, "question_number", req.QuestionNumber)

	result, err := h.svc.SubmitAnswer(r.Context(), req.SessionCode, req.AnswerText, req.QuestionNumber)
	if err != nil {
		slog.Error("answer submission failed", "error", err)
		shared.WriteError(w, shared.ErrInternal, "Internal server error", "INTERNAL_ERROR")
		return
	}

	if result.Done {
		shared.WriteJSON(w, http.StatusOK, answerResponse{Done: true})
		return
	}

	q := result.NextQuestion
	shared.WriteJSON(w, http.StatusOK, answerResponse{
		Done: false,
		NextQuestion: &questionResponse{
			TextEs:         q.TextEs,
			TextEn:         q.TextEn,
			Area:           q.Area,
			QuestionNumber: q.QuestionNumber,
			TotalQuestions: q.TotalQuestions,
		},
	})
}
