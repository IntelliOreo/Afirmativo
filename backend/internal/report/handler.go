// HTTP handlers for report endpoints:
//
//	GET /api/report/{code}     — HandleGetReport (JSON)
//	GET /api/report/{code}/pdf — HandleGetReportPDF (binary PDF download)
package report

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/afirmativo/backend/internal/shared"
)

// Handler holds report HTTP handlers.
type Handler struct {
	svc *Service
}

// NewHandler creates a new report handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// HandleGetReport returns the report JSON for a session.
// Returns 200 if ready, 202 if generating, 404 if session not found.
func (h *Handler) HandleGetReport(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if code == "" {
		shared.WriteJSON(w, http.StatusBadRequest, shared.ErrorResponse{
			Error: "session code is required",
			Code:  "MISSING_CODE",
		})
		return
	}

	report, err := h.svc.GetOrGenerateReport(r.Context(), code)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "not found") {
			shared.WriteJSON(w, http.StatusNotFound, shared.ErrorResponse{
				Error: "session not found",
				Code:  "SESSION_NOT_FOUND",
			})
			return
		}
		if strings.Contains(errStr, "not completed") {
			shared.WriteJSON(w, http.StatusBadRequest, shared.ErrorResponse{
				Error: "interview not completed yet",
				Code:  "NOT_COMPLETED",
			})
			return
		}
		slog.Error("report generation error", "code", code, "error", err)
		shared.WriteJSON(w, http.StatusInternalServerError, shared.ErrorResponse{
			Error: "failed to generate report",
			Code:  "GENERATION_ERROR",
		})
		return
	}

	if report == nil || report.Status == "generating" {
		shared.WriteJSON(w, http.StatusAccepted, map[string]string{
			"status": "generating",
		})
		return
	}

	if report.Status == "failed" {
		shared.WriteJSON(w, http.StatusInternalServerError, shared.ErrorResponse{
			Error: "report generation failed, please try again",
			Code:  "GENERATION_FAILED",
		})
		return
	}

	// Report is ready.
	shared.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"session_code":     report.SessionCode,
		"status":           report.Status,
		"content_en":       report.ContentEn,
		"content_es":       report.ContentEs,
		"strengths":        report.Strengths,
		"weaknesses":       report.Weaknesses,
		"recommendation":   report.Recommendation,
		"question_count":   report.QuestionCount,
		"duration_minutes": report.DurationMinutes,
	})
}

// HandleGetReportPDF serves the report as a PDF download.
// Deferred — returns 501 Not Implemented for now.
func (h *Handler) HandleGetReportPDF(w http.ResponseWriter, r *http.Request) {
	shared.WriteJSON(w, http.StatusNotImplemented, shared.ErrorResponse{
		Error: "PDF generation not implemented yet",
		Code:  "NOT_IMPLEMENTED",
	})
}
