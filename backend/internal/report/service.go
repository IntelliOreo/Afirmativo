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
	sessionCompleted                 = "completed"
	defaultAsyncReportWorkers        = 2
	defaultAsyncReportQueueSize      = 64
	defaultAsyncReportRecoveryBatch  = 50
	defaultAsyncReportRecoveryEvery  = 10 * time.Second
	defaultAsyncReportStaleAfter     = 3 * time.Minute
	defaultAsyncReportJobTimeout     = 3 * time.Minute
	reportAIMaxAttempts              = 3
	reportAIRetryDelay               = 500 * time.Millisecond
)

type AsyncConfig struct {
	Workers       int
	QueueSize     int
	RecoveryBatch int
	RecoveryEvery time.Duration
	StaleAfter    time.Duration
	JobTimeout    time.Duration
}

func (c AsyncConfig) withDefaults() AsyncConfig {
	if c.Workers <= 0 {
		c.Workers = defaultAsyncReportWorkers
	}
	if c.QueueSize <= 0 {
		c.QueueSize = defaultAsyncReportQueueSize
	}
	if c.RecoveryBatch <= 0 {
		c.RecoveryBatch = defaultAsyncReportRecoveryBatch
	}
	if c.RecoveryEvery <= 0 {
		c.RecoveryEvery = defaultAsyncReportRecoveryEvery
	}
	if c.StaleAfter <= 0 {
		c.StaleAfter = defaultAsyncReportStaleAfter
	}
	if c.JobTimeout <= 0 {
		c.JobTimeout = defaultAsyncReportJobTimeout
	}
	return c
}

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
	nowFn       func() time.Time

	asyncWorkers       int
	asyncRecoveryBatch int
	asyncRecoveryEvery time.Duration
	asyncStaleAfter    time.Duration
	asyncJobTimeout    time.Duration
	asyncQueue         chan string
	asyncRuntimeOnce   sync.Once
}

// NewService creates a new report service.
func NewService(store Store, interviews InterviewDataProvider, sessions SessionProvider, aiClient ReportAIClient, areaConfigs []config.AreaConfig) *Service {
	asyncConfig := AsyncConfig{}.withDefaults()
	return &Service{
		store:              store,
		interviews:         interviews,
		sessions:           sessions,
		aiClient:           aiClient,
		areaConfigs:        areaConfigs,
		nowFn:              time.Now,
		asyncWorkers:       asyncConfig.Workers,
		asyncRecoveryBatch: asyncConfig.RecoveryBatch,
		asyncRecoveryEvery: asyncConfig.RecoveryEvery,
		asyncStaleAfter:    asyncConfig.StaleAfter,
		asyncJobTimeout:    asyncConfig.JobTimeout,
		asyncQueue:         make(chan string, asyncConfig.QueueSize),
	}
}

func (s *Service) SetAsyncConfig(cfg AsyncConfig) {
	cfg = cfg.withDefaults()
	s.asyncWorkers = cfg.Workers
	s.asyncRecoveryBatch = cfg.RecoveryBatch
	s.asyncRecoveryEvery = cfg.RecoveryEvery
	s.asyncStaleAfter = cfg.StaleAfter
	s.asyncJobTimeout = cfg.JobTimeout
	if s.asyncQueue == nil || cap(s.asyncQueue) != cfg.QueueSize {
		s.asyncQueue = make(chan string, cfg.QueueSize)
	}
}

func (s *Service) StartAsyncRuntime(ctx context.Context) {
	s.asyncRuntimeOnce.Do(func() {
		if s.asyncQueue == nil {
			s.asyncQueue = make(chan string, defaultAsyncReportQueueSize)
		}

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
			go s.runAsyncWorker(ctx, workerID)
		}
		go s.runAsyncRecoveryLoop(ctx)
	})
}

func (s *Service) runAsyncWorker(ctx context.Context, workerID int) {
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
	requeueCtx, requeueCancel := context.WithTimeout(ctx, 5*time.Second)
	requeued, err := s.store.RequeueStaleRunningReports(requeueCtx, staleBefore)
	requeueCancel()
	if err != nil {
		slog.Error("failed to requeue stale running reports", "error", err)
		return
	}

	listCtx, listCancel := context.WithTimeout(ctx, 5*time.Second)
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

	select {
	case s.asyncQueue <- trimmed:
		slog.Debug("async report queued for worker pickup", "session_code", trimmed)
		return true
	default:
		slog.Warn("async report queue is full; report remains queued", "session_code", trimmed)
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
	claimCtx, claimCancel := context.WithTimeout(ctx, 5*time.Second)
	reportRecord, err := s.store.ClaimQueuedReport(claimCtx, sessionCode)
	claimCancel()
	if err != nil {
		slog.Error("failed to claim queued report", "session_code", sessionCode, "error", err)
		return
	}
	if reportRecord == nil {
		return
	}

	slog.Info("async report claimed",
		"session_code", reportRecord.SessionCode,
		"attempts", reportRecord.Attempts,
	)

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

	successCtx, successCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := s.store.MarkReportReady(successCtx, reportResult); err != nil {
		slog.Error("failed to mark report ready", "session_code", reportRecord.SessionCode, "error", err)
	} else {
		slog.Info("async report marked ready", "session_code", reportRecord.SessionCode)
	}
	successCancel()
}

func (s *Service) markReportFailed(ctx context.Context, sessionCode string, err error) {
	if errors.Is(err, context.Canceled) {
		slog.Info("async report canceled before terminal persistence; leaving report for recovery",
			"session_code", sessionCode,
		)
		return
	}

	errorCode, errorMessage := reportFailureDetails(err)
	failCtx, failCancel := context.WithTimeout(context.Background(), 5*time.Second)
	markErr := s.store.MarkReportFailed(failCtx, sessionCode, errorCode, errorMessage)
	failCancel()
	if markErr != nil {
		slog.Error("failed to mark report failed",
			"session_code", sessionCode,
			"error_code", errorCode,
			"error", markErr,
		)
		return
	}

	slog.Warn("async report marked failed",
		"session_code", sessionCode,
		"error_code", errorCode,
		"error", err,
	)
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

		timer := time.NewTimer(time.Duration(attempt) * reportAIRetryDelay)
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
