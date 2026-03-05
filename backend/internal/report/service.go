// Service layer for report operations.
// GetOrGenerateReport: returns existing report or generates a new one via AI.
package report

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/afirmativo/backend/internal/config"
	"github.com/afirmativo/backend/internal/shared"
)

// Service orchestrates report generation and retrieval.
type Service struct {
	store       Store
	interviews  InterviewDataProvider
	sessions    SessionProvider
	aiClient    AIClient
	areaConfigs []config.AreaConfig
}

// NewService creates a new report service.
func NewService(store Store, interviews InterviewDataProvider, sessions SessionProvider, aiClient AIClient, areaConfigs []config.AreaConfig) *Service {
	return &Service{
		store:       store,
		interviews:  interviews,
		sessions:    sessions,
		aiClient:    aiClient,
		areaConfigs: areaConfigs,
	}
}

// GetOrGenerateReport returns the report if it exists and is ready,
// or generates it synchronously if it doesn't exist yet.
// Returns (report, nil) if ready, (nil, nil) if still generating, or an error.
func (s *Service) GetOrGenerateReport(ctx context.Context, sessionCode string) (*Report, error) {
	// Check if report already exists.
	existing, err := s.store.GetReportBySession(ctx, sessionCode)
	if err != nil {
		return nil, fmt.Errorf("check existing report: %w", err)
	}
	if existing != nil {
		if existing.Status == "ready" {
			return existing, nil
		}
		// Still generating or failed — caller can check status.
		return existing, nil
	}

	// Verify session is completed.
	sess, err := s.sessions.GetSessionByCode(ctx, sessionCode)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("get session: %w", err)
	}
	if sess.Status != "completed" {
		return nil, ErrSessionNotCompleted
	}

	// Create a placeholder row so concurrent requests see "generating".
	placeholder := &Report{
		SessionCode: sessionCode,
		Status:      "generating",
	}
	if err := s.store.CreateReport(ctx, placeholder); err != nil {
		// If another request already created it, fetch and return.
		existing, err2 := s.store.GetReportBySession(ctx, sessionCode)
		if err2 != nil {
			return nil, fmt.Errorf("create report: %w (also: %w)", err, err2)
		}
		return existing, nil
	}

	// Generate the report.
	report, err := s.generateReport(ctx, sessionCode, sess)
	if err != nil {
		slog.Error("report generation failed", "session", sessionCode, "error", err)
		// Update status to failed so user can retry.
		failed := &Report{SessionCode: sessionCode, Status: "failed"}
		_ = s.store.UpdateReport(ctx, failed)
		return nil, fmt.Errorf("generate report: %w", err)
	}

	return report, nil
}

// generateReport builds the area summaries, calls the AI, and persists the result.
func (s *Service) generateReport(ctx context.Context, sessionCode string, sess *SessionInfo) (*Report, error) {
	// 1. Fetch all question areas and answers.
	areas, err := s.interviews.GetAreasBySession(ctx, sessionCode)
	if err != nil {
		return nil, fmt.Errorf("get areas: %w", err)
	}

	answers, err := s.interviews.GetAnswersBySession(ctx, sessionCode)
	if err != nil {
		return nil, fmt.Errorf("get answers: %w", err)
	}

	answerCount, err := s.interviews.GetAnswerCount(ctx, sessionCode)
	if err != nil {
		return nil, fmt.Errorf("get answer count: %w", err)
	}

	// 2. Build area summaries from areas + last evaluation per area.
	summaries := s.buildAreaSummaries(areas, answers)

	// 3. Extract open floor transcript (all answers for the open_floor area).
	openFloorTranscript := s.extractOpenFloorTranscript(answers)

	slog.Debug("report generation context",
		"session", sessionCode,
		"area_count", len(areas),
		"answer_count", answerCount,
		"summaries_count", len(summaries),
		"open_floor_transcript_len", len(openFloorTranscript),
	)

	// 4. Call AI for report generation.
	aiResp, err := s.aiClient.GenerateReport(ctx, summaries, openFloorTranscript)
	if err != nil {
		return nil, fmt.Errorf("AI report generation: %w", err)
	}

	// 5. Calculate duration.
	durationMinutes := 0
	if sess.InterviewStartedAt > 0 && sess.EndedAt > 0 {
		startTime := time.Unix(sess.InterviewStartedAt, 0)
		endTime := time.Unix(sess.EndedAt, 0)
		durationMinutes = int(math.Round(endTime.Sub(startTime).Minutes()))
	}

	// 6. Build and persist report.
	report := &Report{
		SessionCode:     sessionCode,
		Status:          "ready",
		ContentEn:       aiResp.ContentEn,
		ContentEs:       aiResp.ContentEs,
		Strengths:       aiResp.Strengths,
		Weaknesses:      aiResp.Weaknesses,
		Recommendation:  aiResp.Recommendation,
		QuestionCount:   answerCount,
		DurationMinutes: durationMinutes,
	}

	if err := s.store.UpdateReport(ctx, report); err != nil {
		return nil, fmt.Errorf("save report: %w", err)
	}

	slog.Info("report generated",
		"session", sessionCode,
		"strengths", len(report.Strengths),
		"weaknesses", len(report.Weaknesses),
		"duration_min", durationMinutes,
		"questions", answerCount,
	)

	return report, nil
}

// buildAreaSummaries creates compact summaries from areas + answers.
// For each area, finds the last answer's evaluation to get evidence_summary.
func (s *Service) buildAreaSummaries(areas []QuestionAreaRow, answers []AnswerRow) []AreaSummary {
	// Build a map of area slug → last answer's evaluation.
	type evalData struct {
		EvidenceSummary string
		Recommendation  string
	}
	lastEvalByArea := make(map[string]evalData)

	for _, a := range answers {
		if a.AiEvaluation == "" {
			continue
		}
		// Parse the evaluation JSON to extract evidence_summary.
		var eval struct {
			CurrentCriterion struct {
				EvidenceSummary string `json:"evidence_summary"`
				Recommendation  string `json:"recommendation"`
			} `json:"current_criterion"`
		}
		if err := json.Unmarshal([]byte(a.AiEvaluation), &eval); err != nil {
			slog.Warn("failed to parse evaluation for summary", "area", a.Area, "error", err)
			continue
		}
		// Overwrite with each answer — last one wins (answers are ordered by created_at).
		lastEvalByArea[a.Area] = evalData{
			EvidenceSummary: eval.CurrentCriterion.EvidenceSummary,
			Recommendation:  eval.CurrentCriterion.Recommendation,
		}
	}

	// Build summaries from area configs (preserves ordering).
	summaries := make([]AreaSummary, 0, len(areas))
	for _, area := range areas {
		label := area.Area
		for _, cfg := range s.areaConfigs {
			if cfg.Slug == area.Area {
				label = cfg.Label
				break
			}
		}
		ed := lastEvalByArea[area.Area]
		summaries = append(summaries, AreaSummary{
			Slug:            area.Area,
			Label:           label,
			Status:          area.Status,
			EvidenceSummary: ed.EvidenceSummary,
			Recommendation:  ed.Recommendation,
		})
	}
	return summaries
}

// extractOpenFloorTranscript concatenates all answers for the open_floor area.
func (s *Service) extractOpenFloorTranscript(answers []AnswerRow) string {
	var transcript string
	for _, a := range answers {
		if a.Area != "open_floor" {
			continue
		}
		if a.TranscriptEs == "" {
			continue
		}
		if transcript != "" {
			transcript += "\n\n"
		}
		if a.QuestionText != "" {
			transcript += "Q: " + a.QuestionText + "\n"
		}
		transcript += "A: " + a.TranscriptEs
	}
	return transcript
}
