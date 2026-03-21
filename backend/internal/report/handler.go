// HTTP handlers for report endpoints:
//
//	POST /api/report/{code}/generate — HandleGenerateReport (JSON)
//	GET /api/report/{code}     — HandleGetReport (JSON)
//	GET /api/report/{code}/pdf — HandleGetReportPDF (binary PDF download)
package report

import (
	"errors"
	"log/slog"
	"net/http"

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

type reportGeneratingResponse struct {
	Status string `json:"status"`
}

type reportReadyResponse struct {
	SessionCode             string   `json:"session_code"`
	Status                  string   `json:"status"`
	ContentEn               string   `json:"content_en"`
	ContentEs               string   `json:"content_es"`
	AreasOfClarity          []string `json:"areas_of_clarity"`
	AreasOfClarityEs        []string `json:"areas_of_clarity_es"`
	AreasToDevelopFurther   []string `json:"areas_to_develop_further"`
	AreasToDevelopFurtherEs []string `json:"areas_to_develop_further_es"`
	Recommendation          string   `json:"recommendation"`
	RecommendationEs        string   `json:"recommendation_es"`
	QuestionCount           int      `json:"question_count"`
	DurationMinutes         int      `json:"duration_minutes"`
}

func requireReportCode(w http.ResponseWriter, r *http.Request) (string, bool) {
	code := r.PathValue("code")
	if code == "" {
		shared.WriteJSON(w, http.StatusBadRequest, shared.ErrorResponse{
			Error: "session code is required",
			Code:  "MISSING_CODE",
		})
		return "", false
	}
	if !shared.RequireSessionCodeMatch(w, r, code) {
		return "", false
	}
	return code, true
}

// HandleGenerateReport queues report generation or returns the ready report.
func (h *Handler) HandleGenerateReport(w http.ResponseWriter, r *http.Request) {
	code, ok := requireReportCode(w, r)
	if !ok {
		return
	}

	report, err := h.svc.GenerateReportAsync(r.Context(), code)
	if err != nil {
		h.writeServiceError(w, code, err)
		return
	}
	h.writeReportResponse(w, report)
}

// HandleGetReport returns the report JSON for a session.
func (h *Handler) HandleGetReport(w http.ResponseWriter, r *http.Request) {
	code, ok := requireReportCode(w, r)
	if !ok {
		return
	}

	report, err := h.svc.GetReport(r.Context(), code)
	if err != nil {
		h.writeServiceError(w, code, err)
		return
	}

	h.writeReportResponse(w, report)
}

// HandleGetReportPDF serves the report as a PDF download.
// Deferred — returns 501 Not Implemented for now.
func (h *Handler) HandleGetReportPDF(w http.ResponseWriter, r *http.Request) {
	if !shared.RequireSessionCodeMatch(w, r, r.PathValue("code")) {
		return
	}
	shared.WriteJSON(w, http.StatusNotImplemented, shared.ErrorResponse{
		Error: "PDF generation not implemented yet",
		Code:  "NOT_IMPLEMENTED",
	})
}

func (h *Handler) writeServiceError(w http.ResponseWriter, code string, err error) {
	if errors.Is(err, ErrSessionNotFound) {
		shared.WriteJSON(w, http.StatusNotFound, shared.ErrorResponse{
			Error: "session not found",
			Code:  "SESSION_NOT_FOUND",
		})
		return
	}
	if errors.Is(err, ErrSessionNotCompleted) {
		shared.WriteJSON(w, http.StatusBadRequest, shared.ErrorResponse{
			Error: "interview not completed yet",
			Code:  "NOT_COMPLETED",
		})
		return
	}
	if errors.Is(err, ErrReportNotStarted) {
		shared.WriteJSON(w, http.StatusNotFound, shared.ErrorResponse{
			Error: "report has not been started",
			Code:  "REPORT_NOT_STARTED",
		})
		return
	}

	slog.Error("report request failed", "code", code, "error", err)
	shared.WriteJSON(w, http.StatusInternalServerError, shared.ErrorResponse{
		Error: "failed to generate report",
		Code:  "GENERATION_ERROR",
	})
}

func (h *Handler) writeReportResponse(w http.ResponseWriter, report *Report) {
	if report == nil {
		shared.WriteJSON(w, http.StatusNotFound, shared.ErrorResponse{
			Error: "report has not been started",
			Code:  "REPORT_NOT_STARTED",
		})
		return
	}

	if report.Status == ReportStatusQueued || report.Status == ReportStatusRunning || string(report.Status) == "generating" {
		shared.WriteJSON(w, http.StatusAccepted, reportGeneratingResponse{Status: "generating"})
		return
	}

	if report.Status == ReportStatusFailed {
		shared.WriteJSON(w, http.StatusInternalServerError, shared.ErrorResponse{
			Error: "report generation failed, please try again",
			Code:  "GENERATION_FAILED",
		})
		return
	}

	if report.Status != ReportStatusReady {
		slog.Error("report has unexpected status", "session_code", report.SessionCode, "status", report.Status)
		shared.WriteJSON(w, http.StatusInternalServerError, shared.ErrorResponse{
			Error: "failed to generate report",
			Code:  "GENERATION_ERROR",
		})
		return
	}

	shared.WriteJSON(w, http.StatusOK, reportReadyResponse{
		SessionCode:             report.SessionCode,
		Status:                  string(report.Status),
		ContentEn:               report.ContentEn,
		ContentEs:               report.ContentEs,
		AreasOfClarity:          report.AreasOfClarity,
		AreasOfClarityEs:        report.AreasOfClarityEs,
		AreasToDevelopFurther:   report.AreasToDevelopFurther,
		AreasToDevelopFurtherEs: report.AreasToDevelopFurtherEs,
		Recommendation:          report.Recommendation,
		RecommendationEs:        report.RecommendationEs,
		QuestionCount:           report.QuestionCount,
		DurationMinutes:         report.DurationMinutes,
	})
}
