// HTTP handlers for interview endpoints:
//   POST /api/interview/start — HandleStart
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

	result, err := h.svc.StartInterview(r.Context(), req.SessionCode)
	if err != nil {
		switch {
		case errors.Is(err, shared.ErrConflict):
			shared.WriteError(w, shared.ErrConflict, "Interview already started or completed", "INTERVIEW_ALREADY_STARTED")
		case errors.Is(err, shared.ErrNotFound):
			shared.WriteError(w, shared.ErrNotFound, "Session not found", "SESSION_NOT_FOUND")
		default:
			slog.Error("interview start failed", "error", err)
			shared.WriteError(w, shared.ErrInternal, "Internal server error", "INTERNAL_ERROR")
		}
		return
	}

	q := result.Question
	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"question": map[string]any{
			"id":             q.ID,
			"textEs":         q.TextEs,
			"textEn":         q.TextEn,
			"focusArea":      q.FocusArea,
			"questionNumber": q.QuestionNumber,
			"totalQuestions": q.TotalQuestions,
		},
		"timer_remaining_s": result.TimerRemainingS,
	})
}
