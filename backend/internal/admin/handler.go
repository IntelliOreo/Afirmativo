package admin

import (
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/afirmativo/backend/internal/shared"
)

const maxJSONBody = 64 * 1024

// Handler holds admin HTTP handlers.
type Handler struct {
	svc *Service
}

// NewHandler creates a new admin handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

type cleanupDBRequest struct {
	Hours *int `json:"hours"`
}

// HandleCleanupDB runs a DB cleanup job.
// Endpoint: POST /api/admin/cleanup-db
// Body: { "hours": 24 } (optional; defaults to 24)
func (h *Handler) HandleCleanupDB(w http.ResponseWriter, r *http.Request) {
	var req cleanupDBRequest
	if err := shared.DecodeJSON(r, &req, maxJSONBody); err != nil && !errors.Is(err, io.EOF) {
		shared.WriteError(w, shared.ErrBadRequest, "Invalid request body", "BAD_REQUEST")
		return
	}

	result, err := h.svc.CleanupDB(r.Context(), req.Hours)
	if err != nil {
		if errors.Is(err, ErrInvalidHours) {
			shared.WriteError(w, shared.ErrBadRequest, err.Error(), "INVALID_HOURS")
			return
		}
		slog.Error("cleanup job failed", "error", err)
		shared.WriteError(w, shared.ErrInternal, "Cleanup job failed", "CLEANUP_FAILED")
		return
	}

	slog.Info("cleanup job completed",
		"hours", result.Hours,
		"cutoff", result.Cutoff.Format(http.TimeFormat),
		"answers_deleted", result.Deleted.Answers,
		"interview_events_deleted", result.Deleted.InterviewEvents,
		"question_areas_deleted", result.Deleted.QuestionAreas,
		"reports_deleted", result.Deleted.Reports,
		"sessions_deleted", result.Deleted.Sessions,
		"total_deleted", result.TotalDeleted,
	)

	shared.WriteJSON(w, http.StatusOK, result)
}
