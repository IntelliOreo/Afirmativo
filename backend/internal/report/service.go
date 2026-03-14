package report

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/afirmativo/backend/internal/config"
	"github.com/afirmativo/backend/internal/shared"
)

const (
	sessionCompleted       = "completed"
	reportAIMaxAttempts    = 3
	maxReportJobAttempts   = 5
)

// reportAIRetryBackoffs matches the interview pipeline's AI retry backoffs.
var reportAIRetryBackoffs = []time.Duration{3 * time.Second, 7 * time.Second}

type reportGenerationInput struct {
	summaries           []AreaSummary
	openFloorTranscript string
	answerCount         int
	durationMinutes     int
}

type reportAIRetryExhaustedError struct {
	cause error
}

func (e *reportAIRetryExhaustedError) Error() string {
	if e == nil || e.cause == nil {
		return "report AI retry exhausted"
	}
	return fmt.Sprintf("report AI retry exhausted: %v", e.cause)
}

func (e *reportAIRetryExhaustedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Service orchestrates report generation and retrieval.
type Service struct {
	store       Store
	interviews  InterviewDataProvider
	sessions    SessionProvider
	aiClient    ReportAIClient
	areaConfigs []config.AreaConfig
	dbTimeout   time.Duration
	nowFn       func() time.Time

	asyncWorkers          int
	asyncRecoveryBatch    int
	asyncRecoveryEvery    time.Duration
	asyncStaleAfter       time.Duration
	asyncJobTimeout       time.Duration
	asyncQueue            chan string
	asyncRuntimeOnce      sync.Once
	asyncReportRequestIDs sync.Map
	workerWg              sync.WaitGroup
}

type Deps struct {
	Store      Store
	Interviews InterviewDataProvider
	Sessions   SessionProvider
	AIClient   ReportAIClient
}

type Settings struct {
	AreaConfigs  []config.AreaConfig
	DBTimeout    time.Duration
	AsyncRuntime config.AsyncRuntimeConfig
}

// NewService creates a new report service.
func NewService(deps Deps, settings Settings) *Service {
	return &Service{
		store:              deps.Store,
		interviews:         deps.Interviews,
		sessions:           deps.Sessions,
		aiClient:           deps.AIClient,
		areaConfigs:        settings.AreaConfigs,
		dbTimeout:          settings.DBTimeout,
		nowFn:              time.Now,
		asyncWorkers:       settings.AsyncRuntime.Workers,
		asyncRecoveryBatch: settings.AsyncRuntime.RecoveryBatch,
		asyncRecoveryEvery: settings.AsyncRuntime.RecoveryEvery,
		asyncStaleAfter:    settings.AsyncRuntime.StaleAfter,
		asyncJobTimeout:    settings.AsyncRuntime.JobTimeout,
		asyncQueue:         make(chan string, settings.AsyncRuntime.QueueSize),
	}
}

// HealthStats returns async runtime stats for the health endpoint.
func (s *Service) HealthStats() map[string]any {
	return map[string]any{
		"async_report_queue_depth":    len(s.asyncQueue),
		"async_report_queue_capacity": cap(s.asyncQueue),
		"async_report_workers":        s.asyncWorkers,
	}
}

func (s *Service) StartAsyncRuntime(ctx context.Context) {
	s.asyncRuntimeOnce.Do(func() {
		slog.Info("starting async report runtime",
			"workers", s.asyncWorkers,
			"queue_size", cap(s.asyncQueue),
			"recovery_batch", s.asyncRecoveryBatch,
			"recovery_every", s.asyncRecoveryEvery,
			"stale_after", s.asyncStaleAfter,
			"job_timeout", s.asyncJobTimeout,
		)

		for i := 0; i < s.asyncWorkers; i++ {
			workerID := i + 1
			s.workerWg.Add(1)
			go s.runAsyncWorker(ctx, workerID)
		}
		go s.runAsyncRecoveryLoop(ctx)
	})
}

// WaitForDrain blocks until all async report workers have exited or the
// context deadline is reached. Call after cancelling the runtime context.
func (s *Service) WaitForDrain(ctx context.Context) {
	shared.WaitForWorkers(ctx, &s.workerWg, "async report")
}

func (s *Service) runAsyncWorker(ctx context.Context, workerID int) {
	defer s.workerWg.Done()
	slog.Debug("async report worker started", "worker_id", workerID)
	defer slog.Debug("async report worker stopped", "worker_id", workerID)

	for {
		select {
		case <-ctx.Done():
			return
		case sessionCode := <-s.asyncQueue:
			processCtx, cancel := context.WithTimeout(ctx, s.asyncJobTimeout)
			s.processQueuedReport(processCtx, sessionCode)
			cancel()
		}
	}
}

func (s *Service) runAsyncRecoveryLoop(ctx context.Context) {
	s.recoverAsyncReports(ctx)

	ticker := time.NewTicker(s.asyncRecoveryEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.recoverAsyncReports(ctx)
		}
	}
}

func (s *Service) recoverAsyncReports(ctx context.Context) {
	staleBefore := s.nowFn().UTC().Add(-s.asyncStaleAfter)
	requeueCtx, requeueCancel := context.WithTimeout(ctx, s.dbTimeout)
	requeued, err := s.store.RequeueStaleRunningReports(requeueCtx, staleBefore)
	requeueCancel()
	if err != nil {
		slog.Error("failed to requeue stale running reports", "error", err)
		return
	}

	listCtx, listCancel := context.WithTimeout(ctx, s.dbTimeout)
	queuedSessionCodes, err := s.store.ListQueuedReportSessionCodes(listCtx, s.asyncRecoveryBatch)
	listCancel()
	if err != nil {
		slog.Error("failed to list queued reports", "error", err)
		return
	}

	enqueued := 0
	for _, sessionCode := range queuedSessionCodes {
		if s.enqueueReport(sessionCode) {
			enqueued++
		}
	}

	if requeued > 0 || enqueued > 0 {
		slog.Info("async report recovery cycle completed",
			"requeued_stale_running_reports", requeued,
			"queued_reports_listed", len(queuedSessionCodes),
			"queued_reports_enqueued", enqueued,
		)
	}
}

func (s *Service) enqueueReport(sessionCode string) bool {
	trimmed := strings.TrimSpace(sessionCode)
	if trimmed == "" || s.asyncQueue == nil {
		return false
	}
	requestID := s.asyncReportRequestID(trimmed)

	select {
	case s.asyncQueue <- trimmed:
		slog.Debug("async report queued for worker pickup", "session_code", trimmed, "request_id", requestID)
		return true
	default:
		slog.Warn("async report queue is full; report remains queued", "session_code", trimmed, "request_id", requestID)
		return false
	}
}

func (s *Service) GetReport(ctx context.Context, sessionCode string) (*Report, error) {
	existing, err := s.store.GetReportBySession(ctx, sessionCode)
	if err != nil {
		return nil, fmt.Errorf("check existing report: %w", err)
	}
	if existing != nil {
		return existing, nil
	}

	if _, err := s.getCompletedSession(ctx, sessionCode); err != nil {
		return nil, err
	}
	return nil, ErrReportNotStarted
}

func (s *Service) GenerateReportAsync(ctx context.Context, sessionCode string) (*Report, error) {
	if requestID := shared.RequestIDFromContext(ctx); requestID != "" {
		s.asyncReportRequestIDs.Store(strings.TrimSpace(sessionCode), requestID)
	}
	if _, err := s.getCompletedSession(ctx, sessionCode); err != nil {
		return nil, err
	}

	existing, err := s.store.GetReportBySession(ctx, sessionCode)
	if err != nil {
		return nil, fmt.Errorf("check existing report: %w", err)
	}

	if existing != nil {
		if string(existing.Status) == "generating" {
			if err := s.store.SetReportQueued(ctx, sessionCode, false); err != nil {
				return nil, fmt.Errorf("queue legacy generating report: %w", err)
			}
			s.enqueueReport(sessionCode)
			existing.Status = ReportStatusQueued
			return existing, nil
		}

		switch existing.Status {
		case ReportStatusReady:
			return existing, nil
		case ReportStatusQueued, ReportStatusRunning:
			s.enqueueReport(sessionCode)
			return existing, nil
		case ReportStatusFailed:
			if err := s.store.SetReportQueued(ctx, sessionCode, true); err != nil {
				return nil, fmt.Errorf("queue failed report: %w", err)
			}
			s.enqueueReport(sessionCode)
			existing.Status = ReportStatusQueued
			existing.ErrorCode = ""
			existing.ErrorMessage = ""
			existing.CompletedAt = nil
			existing.StartedAt = nil
			existing.Attempts = 0
			return existing, nil
		}
	}

	placeholder := &Report{
		SessionCode: sessionCode,
		Status:      ReportStatusQueued,
	}
	if err := s.store.CreateReport(ctx, placeholder); err != nil {
		existing, err2 := s.store.GetReportBySession(ctx, sessionCode)
		if err2 != nil {
			return nil, fmt.Errorf("create report: %w (also: %w)", err, err2)
		}
		if existing != nil {
			s.enqueueReport(sessionCode)
			return existing, nil
		}
		return nil, fmt.Errorf("create report: %w", err)
	}

	s.enqueueReport(sessionCode)
	return placeholder, nil
}

func (s *Service) processQueuedReport(ctx context.Context, sessionCode string) {
	claimCtx, claimCancel := context.WithTimeout(ctx, s.dbTimeout)
	reportRecord, err := s.store.ClaimQueuedReport(claimCtx, sessionCode)
	claimCancel()
	requestID := s.asyncReportRequestID(sessionCode)
	if err != nil {
		slog.Error("failed to claim queued report", "session_code", sessionCode, "request_id", requestID, "error", err)
		return
	}
	if reportRecord == nil {
		return
	}
	defer s.asyncReportRequestIDs.Delete(reportRecord.SessionCode)

	slog.Info("async report claimed",
		"request_id", requestID,
		"session_code", reportRecord.SessionCode,
		"attempts", reportRecord.Attempts,
	)

	if reportRecord.Attempts > maxReportJobAttempts {
		slog.Error("async report exceeded max attempts",
			"request_id", requestID,
			"session_code", reportRecord.SessionCode,
			"attempts", reportRecord.Attempts,
		)
		s.markReportFailed(ctx, reportRecord.SessionCode, fmt.Errorf("report generation failed after %d attempts", reportRecord.Attempts))
		return
	}

	sess, err := s.getCompletedSession(ctx, reportRecord.SessionCode)
	if err != nil {
		s.markReportFailed(ctx, reportRecord.SessionCode, err)
		return
	}

	input, err := s.buildReportInput(ctx, reportRecord.SessionCode, sess)
	if err != nil {
		s.markReportFailed(ctx, reportRecord.SessionCode, err)
		return
	}

	reportResult, err := s.generateReportWithRetry(ctx, reportRecord.SessionCode, input)
	if err != nil {
		s.markReportFailed(ctx, reportRecord.SessionCode, err)
		return
	}

	successCtx, successCancel := context.WithTimeout(context.Background(), s.dbTimeout)
	if err := s.store.MarkReportReady(successCtx, reportResult); err != nil {
		successCancel()
		slog.Error("failed to mark report ready", "session_code", reportRecord.SessionCode, "request_id", requestID, "error", err)
		s.markReportFailed(ctx, reportRecord.SessionCode, fmt.Errorf("persist ready failed: %w", err))
		return
	}
	successCancel()
	slog.Info("async report marked ready", "session_code", reportRecord.SessionCode, "request_id", requestID)
}

func (s *Service) markReportFailed(ctx context.Context, sessionCode string, err error) {
	if errors.Is(err, context.Canceled) {
		slog.Info("async report canceled before terminal persistence; leaving report for recovery",
			"session_code", sessionCode,
			"request_id", s.asyncReportRequestID(sessionCode),
		)
		return
	}

	errorCode, errorMessage := reportFailureDetails(err)
	failCtx, failCancel := context.WithTimeout(context.Background(), s.dbTimeout)
	markErr := s.store.MarkReportFailed(failCtx, sessionCode, errorCode, errorMessage)
	failCancel()
	if markErr != nil {
		slog.Error("failed to mark report failed",
			"session_code", sessionCode,
			"request_id", s.asyncReportRequestID(sessionCode),
			"error_code", errorCode,
			"error", markErr,
		)
		return
	}

	slog.Warn("async report marked failed",
		"session_code", sessionCode,
		"request_id", s.asyncReportRequestID(sessionCode),
		"error_code", errorCode,
		"error", err,
	)
}

func (s *Service) asyncReportRequestID(sessionCode string) string {
	value, ok := s.asyncReportRequestIDs.Load(strings.TrimSpace(sessionCode))
	if !ok {
		return ""
	}
	requestID, _ := value.(string)
	return strings.TrimSpace(requestID)
}

func reportFailureDetails(err error) (string, string) {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "JOB_TIMEOUT", "Report generation timed out"
	case errors.Is(err, context.Canceled):
		return "JOB_CANCELED", "Report generation was canceled"
	case errors.Is(err, ErrSessionNotCompleted):
		return "NOT_COMPLETED", "Interview is not completed"
	case errors.Is(err, ErrSessionNotFound):
		return "SESSION_NOT_FOUND", "Session not found"
	}

	var retryErr *reportAIRetryExhaustedError
	if errors.As(err, &retryErr) {
		return "AI_RETRY_EXHAUSTED", "AI report generation failed after retries"
	}
	if strings.Contains(err.Error(), "persist ready failed") {
		return "PERSIST_READY_FAILED", "Failed to persist report after successful generation"
	}

	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "Report generation failed"
	}
	return "GENERATION_ERROR", message
}

func (s *Service) getCompletedSession(ctx context.Context, sessionCode string) (*SessionInfo, error) {
	sess, err := s.sessions.GetSessionByCode(ctx, sessionCode)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("get session: %w", err)
	}
	if sess == nil || sess.Status != sessionCompleted {
		return nil, ErrSessionNotCompleted
	}
	return sess, nil
}

func (s *Service) buildReportInput(ctx context.Context, sessionCode string, sess *SessionInfo) (*reportGenerationInput, error) {
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

	summaries := s.buildAreaSummaries(areas, answers)
	openFloorTranscript := s.extractOpenFloorTranscript(answers, sess.PreferredLanguage)

	durationMinutes := 0
	if sess.InterviewStartedAt > 0 && sess.EndedAt > 0 {
		startTime := time.Unix(sess.InterviewStartedAt, 0)
		endTime := time.Unix(sess.EndedAt, 0)
		durationMinutes = int(math.Round(endTime.Sub(startTime).Minutes()))
	}

	slog.Debug("report generation context",
		"session", sessionCode,
		"area_count", len(areas),
		"answer_count", answerCount,
		"summaries_count", len(summaries),
		"open_floor_transcript_len", len(openFloorTranscript),
	)

	return &reportGenerationInput{
		summaries:           summaries,
		openFloorTranscript: openFloorTranscript,
		answerCount:         answerCount,
		durationMinutes:     durationMinutes,
	}, nil
}

func (s *Service) generateReportWithRetry(ctx context.Context, sessionCode string, input *reportGenerationInput) (*Report, error) {
	var lastErr error
	for attempt := 1; attempt <= reportAIMaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		aiResp, err := s.aiClient.GenerateReport(ctx, input.summaries, input.openFloorTranscript)
		if err == nil {
			report := &Report{
				SessionCode:             sessionCode,
				Status:                  ReportStatusReady,
				ContentEn:               aiResp.ContentEn,
				ContentEs:               aiResp.ContentEs,
				AreasOfClarity:          aiResp.AreasOfClarity,
				AreasOfClarityEs:        aiResp.AreasOfClarityEs,
				AreasToDevelopFurther:   aiResp.AreasToDevelopFurther,
				AreasToDevelopFurtherEs: aiResp.AreasToDevelopFurtherEs,
				Recommendation:          aiResp.Recommendation,
				RecommendationEs:        aiResp.RecommendationEs,
				QuestionCount:           input.answerCount,
				DurationMinutes:         input.durationMinutes,
			}

			slog.Info("report generated",
				"session", sessionCode,
				"areas_of_clarity", len(report.AreasOfClarity),
				"areas_to_develop_further", len(report.AreasToDevelopFurther),
				"duration_min", input.durationMinutes,
				"questions", input.answerCount,
				"attempt", attempt,
			)
			return report, nil
		}

		lastErr = err
		if attempt == reportAIMaxAttempts {
			break
		}

		backoff := reportAIRetryBackoffs[len(reportAIRetryBackoffs)-1]
		if idx := attempt - 1; idx < len(reportAIRetryBackoffs) {
			backoff = reportAIRetryBackoffs[idx]
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}

	return nil, &reportAIRetryExhaustedError{cause: lastErr}
}

// buildAreaSummaries creates compact summaries from areas + answers.
// For each area, finds the last answer's evaluation to get evidence_summary.
func (s *Service) buildAreaSummaries(areas []InterviewAreaSnapshot, answers []InterviewAnswerSnapshot) []AreaSummary {
	type evalData struct {
		EvidenceSummary string
		Recommendation  string
	}
	lastEvalByArea := make(map[string]evalData)

	for _, a := range answers {
		if a.AIEvaluation == nil {
			continue
		}
		lastEvalByArea[a.Area] = evalData{
			EvidenceSummary: a.AIEvaluation.EvidenceSummary,
			Recommendation:  a.AIEvaluation.Recommendation,
		}
	}

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
func (s *Service) extractOpenFloorTranscript(answers []InterviewAnswerSnapshot, preferredLanguage string) string {
	useEnglish := strings.EqualFold(strings.TrimSpace(preferredLanguage), "en")

	var transcript string
	for _, a := range answers {
		if a.Area != "open_floor" {
			continue
		}

		answerText := strings.TrimSpace(a.TranscriptEs)
		if useEnglish {
			answerText = strings.TrimSpace(a.TranscriptEn)
			if answerText == "" {
				answerText = strings.TrimSpace(a.TranscriptEs)
			}
		} else if answerText == "" {
			answerText = strings.TrimSpace(a.TranscriptEn)
		}

		if answerText == "" {
			continue
		}
		if transcript != "" {
			transcript += "\n\n"
		}
		if a.QuestionText != "" {
			transcript += "Q: " + a.QuestionText + "\n"
		}
		transcript += "A: " + answerText
	}
	return transcript
}
