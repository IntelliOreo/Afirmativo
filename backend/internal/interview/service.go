// Service layer for interview operations.
// StartInterview: sets session to interviewing, creates areas, returns first question.
// SubmitAnswer: persists answer, evaluates via AI, manages area transitions.
package interview

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/afirmativo/backend/internal/config"
	"github.com/afirmativo/backend/internal/session"
)

const dbTimeout = 5 * time.Second
const asyncAnswerJobTimeout = 3 * time.Minute

const (
	defaultAsyncAnswerWorkers         = 4
	defaultAsyncAnswerQueueSize       = 256
	defaultAsyncAnswerRecoveryBatch   = 100
	defaultAsyncAnswerRecoveryEvery   = 10 * time.Second
	defaultAsyncAnswerStaleRunningAge = 4 * time.Minute
)

// SessionStarter transitions a session to 'interviewing'.
type SessionStarter interface {
	StartSession(ctx context.Context, sessionCode, preferredLanguage string) (*session.Session, error)
}

// SessionGetter retrieves session data (for timer calculation).
type SessionGetter interface {
	GetSessionByCode(ctx context.Context, sessionCode string) (*session.Session, error)
}

// SessionCompleter marks a session as completed.
type SessionCompleter interface {
	CompleteSession(ctx context.Context, sessionCode string) error
}

// AIClient calls the AI API to evaluate answers and generate next questions.
type AIClient interface {
	CallAI(ctx context.Context, turnCtx *AITurnContext) (*AIResponse, error)
}

// Service contains interview business logic.
type Service struct {
	sessionStarter   SessionStarter
	sessionGetter    SessionGetter
	sessionCompleter SessionCompleter
	store            Store
	aiClient         AIClient
	areaConfigs      []config.AreaConfig
	openingTextEn    string
	openingTextEs    string
	readinessTextEn  string
	readinessTextEs  string

	asyncAnswerWorkers       int
	asyncAnswerRecoveryBatch int
	asyncAnswerRecoveryEvery time.Duration
	asyncAnswerStaleAfter    time.Duration
	asyncAnswerQueue         chan string
	asyncRuntimeStartOnce    sync.Once
}

// NewService creates a Service with the given dependencies.
func NewService(
	ss SessionStarter,
	sg SessionGetter,
	sc SessionCompleter,
	store Store,
	ai AIClient,
	areaConfigs []config.AreaConfig,
	openingTextEn, openingTextEs, readinessTextEn, readinessTextEs string,
) *Service {
	svc := &Service{
		sessionStarter:           ss,
		sessionGetter:            sg,
		sessionCompleter:         sc,
		store:                    store,
		aiClient:                 ai,
		areaConfigs:              areaConfigs,
		openingTextEn:            openingTextEn,
		openingTextEs:            openingTextEs,
		readinessTextEn:          readinessTextEn,
		readinessTextEs:          readinessTextEs,
		asyncAnswerWorkers:       defaultAsyncAnswerWorkers,
		asyncAnswerRecoveryBatch: defaultAsyncAnswerRecoveryBatch,
		asyncAnswerRecoveryEvery: defaultAsyncAnswerRecoveryEvery,
		asyncAnswerStaleAfter:    defaultAsyncAnswerStaleRunningAge,
	}
	svc.asyncAnswerQueue = make(chan string, defaultAsyncAnswerQueueSize)
	return svc
}

// ConfigureAsyncAnswerRuntime overrides async job worker/recovery settings.
func (s *Service) ConfigureAsyncAnswerRuntime(
	workers int,
	queueSize int,
	recoveryBatch int,
	recoveryEvery time.Duration,
	staleAfter time.Duration,
) {
	if workers <= 0 {
		workers = defaultAsyncAnswerWorkers
	}
	if queueSize <= 0 {
		queueSize = defaultAsyncAnswerQueueSize
	}
	if recoveryBatch <= 0 {
		recoveryBatch = defaultAsyncAnswerRecoveryBatch
	}
	if recoveryEvery <= 0 {
		recoveryEvery = defaultAsyncAnswerRecoveryEvery
	}
	if staleAfter <= 0 {
		staleAfter = defaultAsyncAnswerStaleRunningAge
	}

	s.asyncAnswerWorkers = workers
	s.asyncAnswerRecoveryBatch = recoveryBatch
	s.asyncAnswerRecoveryEvery = recoveryEvery
	s.asyncAnswerStaleAfter = staleAfter
	s.asyncAnswerQueue = make(chan string, queueSize)
}

// StartAsyncAnswerRuntime launches bounded workers and periodic recovery.
func (s *Service) StartAsyncAnswerRuntime(ctx context.Context) {
	s.asyncRuntimeStartOnce.Do(func() {
		if s.asyncAnswerQueue == nil {
			s.asyncAnswerQueue = make(chan string, defaultAsyncAnswerQueueSize)
		}

		slog.Info("starting async answer runtime",
			"workers", s.asyncAnswerWorkers,
			"queue_size", cap(s.asyncAnswerQueue),
			"recovery_batch", s.asyncAnswerRecoveryBatch,
			"recovery_every", s.asyncAnswerRecoveryEvery,
			"stale_after", s.asyncAnswerStaleAfter,
		)

		for i := 0; i < s.asyncAnswerWorkers; i++ {
			workerID := i + 1
			go s.runAsyncAnswerWorker(ctx, workerID)
		}
		go s.runAsyncAnswerRecoveryLoop(ctx)
	})
}

func (s *Service) runAsyncAnswerWorker(ctx context.Context, workerID int) {
	slog.Debug("async answer worker started", "worker_id", workerID)
	defer slog.Debug("async answer worker stopped", "worker_id", workerID)

	for {
		select {
		case <-ctx.Done():
			return
		case jobID := <-s.asyncAnswerQueue:
			processCtx, cancel := context.WithTimeout(ctx, asyncAnswerJobTimeout)
			s.processAnswerJob(processCtx, jobID)
			cancel()
		}
	}
}

func (s *Service) runAsyncAnswerRecoveryLoop(ctx context.Context) {
	s.recoverAsyncAnswerJobs(ctx)

	ticker := time.NewTicker(s.asyncAnswerRecoveryEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.recoverAsyncAnswerJobs(ctx)
		}
	}
}

func (s *Service) recoverAsyncAnswerJobs(ctx context.Context) {
	staleBefore := time.Now().UTC().Add(-s.asyncAnswerStaleAfter)
	requeueCtx, requeueCancel := context.WithTimeout(ctx, dbTimeout)
	requeued, err := s.store.RequeueStaleRunningAnswerJobs(requeueCtx, staleBefore)
	requeueCancel()
	if err != nil {
		slog.Error("failed to requeue stale running async answer jobs", "error", err)
		return
	}

	listCtx, listCancel := context.WithTimeout(ctx, dbTimeout)
	queuedIDs, err := s.store.ListQueuedAnswerJobIDs(listCtx, s.asyncAnswerRecoveryBatch)
	listCancel()
	if err != nil {
		slog.Error("failed to list queued async answer jobs", "error", err)
		return
	}

	enqueued := 0
	for _, jobID := range queuedIDs {
		if s.enqueueAsyncAnswerJob(jobID) {
			enqueued++
		}
	}

	if requeued > 0 || enqueued > 0 {
		slog.Info("async answer recovery cycle completed",
			"requeued_stale_running_jobs", requeued,
			"queued_jobs_listed", len(queuedIDs),
			"queued_jobs_enqueued", enqueued,
		)
	}
}

func (s *Service) enqueueAsyncAnswerJob(jobID string) bool {
	id := strings.TrimSpace(jobID)
	if id == "" {
		return false
	}
	if s.asyncAnswerQueue == nil {
		slog.Warn("async answer queue is not configured; job remains queued", "job_id", id)
		return false
	}

	select {
	case s.asyncAnswerQueue <- id:
		slog.Debug("async answer job queued for worker pickup", "job_id", id)
		return true
	default:
		// Leave status as queued in DB; recovery loop will retry enqueue.
		slog.Warn("async answer queue is full; job remains queued", "job_id", id)
		return false
	}
}

// StartResult holds the output of a successful interview start.
type StartResult struct {
	Question        *Question
	TimerRemainingS int
	Area            string
	Resuming        bool
	Language        string
}

// StartInterview transitions the session to interviewing,
// creates all question area rows, and returns the opening question.
func (s *Service) StartInterview(ctx context.Context, sessionCode, preferredLanguage string) (*StartResult, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	existing, err := s.sessionGetter.GetSessionByCode(dbCtx, sessionCode)
	if err != nil {
		return nil, err
	}
	if time.Now().After(existing.ExpiresAt) {
		slog.Debug("start interview rejected: session expired", "session_code", sessionCode)
		return nil, session.ErrSessionExpired
	}

	sess, err := s.sessionStarter.StartSession(dbCtx, sessionCode, preferredLanguage)
	if err != nil {
		return nil, err
	}
	effectiveLanguage := normalizePreferredLanguage(sess.PreferredLanguage)

	remaining := sess.InterviewBudgetSeconds - sess.InterviewLapsedSeconds

	answersCount, err := s.store.GetAnswerCount(dbCtx, sessionCode)
	if err != nil {
		return nil, fmt.Errorf("get answer count: %w", err)
	}

	// Resume only after there is actual prior progress. This avoids showing
	// "Welcome back" on first entry if the start endpoint is called twice.
	resuming := answersCount > 0

	// Pre-create all 8 question area rows (idempotent — ON CONFLICT DO NOTHING).
	for _, area := range s.areaConfigs {
		if _, err := s.store.CreateQuestionArea(dbCtx, sessionCode, area.Slug); err != nil {
			return nil, fmt.Errorf("create question area %s: %w", area.Slug, err)
		}
	}

	// Set the first area to in_progress (no-op if already in_progress from a prior start).
	firstArea := s.areaConfigs[0].Slug
	if err := s.store.SetAreaInProgress(dbCtx, sessionCode, firstArea); err != nil {
		return nil, fmt.Errorf("set first area in_progress: %w", err)
	}

	turnID, err := newTurnID()
	if err != nil {
		return nil, fmt.Errorf("new turn id: %w", err)
	}
	flowState, err := s.store.PrepareDisclaimerStep(dbCtx, sessionCode, turnID)
	if err != nil {
		return nil, fmt.Errorf("prepare disclaimer step: %w", err)
	}

	q := OpeningDisclaimerQuestion(
		firstArea,
		s.openingTextEs,
		s.openingTextEn,
		flowState.QuestionNumber,
		turnID,
	)

	return &StartResult{
		Question:        q,
		TimerRemainingS: remaining,
		Area:            firstArea,
		Resuming:        resuming,
		Language:        effectiveLanguage,
	}, nil
}

// AnswerResult holds the output of a submitted answer.
type AnswerResult struct {
	Done            bool
	NextQuestion    *Question
	TimerRemainingS int
}

// SubmitAnswerAsyncResult is returned when an async answer job is accepted.
type SubmitAnswerAsyncResult struct {
	JobID           string
	ClientRequestID string
	Status          AsyncAnswerJobStatus
}

// AnswerJobStatusResult is returned by polling for async answer job state.
type AnswerJobStatusResult struct {
	JobID           string
	ClientRequestID string
	Status          AsyncAnswerJobStatus
	Done            bool
	NextQuestion    *Question
	TimerRemainingS int
	ErrorCode       string
	ErrorMessage    string
}

type answerJobPayload struct {
	Done            bool                    `json:"done"`
	NextQuestion    *answerJobQuestionShape `json:"nextQuestion"`
	TimerRemainingS int                     `json:"timerRemainingS"`
}

type answerJobQuestionShape struct {
	TextEs         string `json:"textEs"`
	TextEn         string `json:"textEn"`
	Area           string `json:"area"`
	Kind           string `json:"kind"`
	TurnID         string `json:"turnId"`
	QuestionNumber int    `json:"questionNumber"`
	TotalQuestions int    `json:"totalQuestions"`
}

// SubmitAnswerAsync queues one async answer job and starts background processing.
func (s *Service) SubmitAnswerAsync(ctx context.Context, sessionCode, answerText, questionText, turnID, clientRequestID string) (*SubmitAnswerAsyncResult, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	sess, err := s.sessionGetter.GetSessionByCode(dbCtx, sessionCode)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	if time.Now().After(sess.ExpiresAt) {
		slog.Debug("submit answer async rejected: session expired", "session_code", sessionCode)
		return nil, session.ErrSessionExpired
	}

	job, err := s.store.UpsertAnswerJob(dbCtx, UpsertAnswerJobParams{
		SessionCode:     sessionCode,
		ClientRequestID: clientRequestID,
		TurnID:          turnID,
		QuestionText:    questionText,
		AnswerText:      answerText,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert async answer job: %w", err)
	}

	// Idempotency key must map to the same semantic request payload.
	if strings.TrimSpace(job.TurnID) != strings.TrimSpace(turnID) ||
		strings.TrimSpace(job.AnswerText) != strings.TrimSpace(answerText) ||
		strings.TrimSpace(job.QuestionText) != strings.TrimSpace(questionText) {
		slog.Warn("async answer idempotency conflict",
			"session_code", sessionCode,
			"client_request_id", clientRequestID,
			"provided_turn_id", turnID,
			"stored_turn_id", job.TurnID,
		)
		return nil, ErrIdempotencyConflict
	}

	triggered := false
	if job.Status == AsyncAnswerJobQueued {
		triggered = s.enqueueAsyncAnswerJob(job.ID)
	}

	slog.Info("async answer job accepted",
		"session_code", job.SessionCode,
		"client_request_id", job.ClientRequestID,
		"job_id", job.ID,
		"status", job.Status,
		"attempts", job.Attempts,
		"worker_triggered", triggered,
	)

	return &SubmitAnswerAsyncResult{
		JobID:           job.ID,
		ClientRequestID: job.ClientRequestID,
		Status:          job.Status,
	}, nil
}

// GetAnswerJobResult returns current async job status and, when available, the computed next question payload.
func (s *Service) GetAnswerJobResult(ctx context.Context, sessionCode, jobID string) (*AnswerJobStatusResult, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	job, err := s.store.GetAnswerJob(dbCtx, sessionCode, jobID)
	if err != nil {
		return nil, err
	}

	slog.Debug("async answer job fetched",
		"session_code", sessionCode,
		"job_id", job.ID,
		"client_request_id", job.ClientRequestID,
		"status", job.Status,
		"attempts", job.Attempts,
	)

	result := &AnswerJobStatusResult{
		JobID:           job.ID,
		ClientRequestID: job.ClientRequestID,
		Status:          job.Status,
		ErrorCode:       job.ErrorCode,
		ErrorMessage:    job.ErrorMessage,
	}

	if job.Status != AsyncAnswerJobSucceeded || len(job.ResultPayload) == 0 {
		return result, nil
	}

	var payload answerJobPayload
	if err := json.Unmarshal(job.ResultPayload, &payload); err != nil {
		return nil, fmt.Errorf("decode answer job payload: %w", err)
	}

	result.Done = payload.Done
	result.TimerRemainingS = payload.TimerRemainingS
	if payload.NextQuestion != nil {
		result.NextQuestion = &Question{
			TextEs:         payload.NextQuestion.TextEs,
			TextEn:         payload.NextQuestion.TextEn,
			Area:           payload.NextQuestion.Area,
			Kind:           QuestionKind(payload.NextQuestion.Kind),
			TurnID:         payload.NextQuestion.TurnID,
			QuestionNumber: payload.NextQuestion.QuestionNumber,
			TotalQuestions: payload.NextQuestion.TotalQuestions,
		}
	}

	return result, nil
}

func (s *Service) processAnswerJob(ctx context.Context, jobID string) {
	claimCtx, claimCancel := context.WithTimeout(ctx, dbTimeout)
	job, err := s.store.ClaimQueuedAnswerJob(claimCtx, jobID)
	claimCancel()
	if err != nil {
		slog.Error("failed to claim async answer job", "job_id", jobID, "error", err)
		return
	}
	if job == nil {
		slog.Debug("async answer job not claimable", "job_id", jobID)
		return
	}

	slog.Info("async answer job claimed",
		"session_code", job.SessionCode,
		"client_request_id", job.ClientRequestID,
		"job_id", job.ID,
		"status", job.Status,
		"attempts", job.Attempts,
	)

	answerResult, err := s.SubmitAnswer(ctx, job.SessionCode, job.AnswerText, job.QuestionText, job.TurnID)
	if err != nil {
		status := AsyncAnswerJobFailed
		errorCode := "INTERNAL_ERROR"
		errorMessage := "Internal server error"
		if errors.Is(err, ErrTurnConflict) {
			status = AsyncAnswerJobConflict
			errorCode = "TURN_CONFLICT"
			errorMessage = "Turn is stale or out of order"
		} else if errors.Is(err, ErrInvalidFlow) {
			errorCode = "FLOW_INVALID"
			errorMessage = "Interview flow is not in a valid state"
		}

		slog.Warn("async answer job processing failed",
			"session_code", job.SessionCode,
			"client_request_id", job.ClientRequestID,
			"job_id", job.ID,
			"status", status,
			"error_code", errorCode,
			"error", err,
		)

		failCtx, failCancel := context.WithTimeout(ctx, dbTimeout)
		markErr := s.store.MarkAnswerJobFailed(failCtx, MarkAnswerJobFailedParams{
			JobID:        job.ID,
			Status:       status,
			ErrorCode:    errorCode,
			ErrorMessage: errorMessage,
		})
		failCancel()
		if markErr != nil {
			slog.Error("failed to mark async answer job as failed",
				"session_code", job.SessionCode,
				"client_request_id", job.ClientRequestID,
				"job_id", job.ID,
				"status", status,
				"error_code", errorCode,
				"error", markErr,
			)
		} else {
			slog.Info("async answer job marked terminal",
				"session_code", job.SessionCode,
				"client_request_id", job.ClientRequestID,
				"job_id", job.ID,
				"status", status,
				"error_code", errorCode,
			)
		}
		return
	}

	payload, err := json.Marshal(toAnswerJobPayload(answerResult))
	if err != nil {
		failCtx, failCancel := context.WithTimeout(ctx, dbTimeout)
		markErr := s.store.MarkAnswerJobFailed(failCtx, MarkAnswerJobFailedParams{
			JobID:        job.ID,
			Status:       AsyncAnswerJobFailed,
			ErrorCode:    "SERIALIZATION_ERROR",
			ErrorMessage: "Failed to serialize async job result",
		})
		failCancel()
		if markErr != nil {
			slog.Error("failed to mark async answer job serialization failure",
				"session_code", job.SessionCode,
				"client_request_id", job.ClientRequestID,
				"job_id", job.ID,
				"error", markErr,
			)
		} else {
			slog.Warn("async answer job serialization failure",
				"session_code", job.SessionCode,
				"client_request_id", job.ClientRequestID,
				"job_id", job.ID,
				"error_code", "SERIALIZATION_ERROR",
			)
		}
		return
	}

	successCtx, successCancel := context.WithTimeout(ctx, dbTimeout)
	if err := s.store.MarkAnswerJobSucceeded(successCtx, job.ID, payload); err != nil {
		slog.Error("failed to mark async answer job as succeeded",
			"session_code", job.SessionCode,
			"client_request_id", job.ClientRequestID,
			"job_id", job.ID,
			"error", err,
		)
	} else {
		slog.Info("async answer job marked terminal",
			"session_code", job.SessionCode,
			"client_request_id", job.ClientRequestID,
			"job_id", job.ID,
			"status", AsyncAnswerJobSucceeded,
		)
	}
	successCancel()
}

func toAnswerJobPayload(result *AnswerResult) *answerJobPayload {
	payload := &answerJobPayload{
		Done:            result.Done,
		TimerRemainingS: result.TimerRemainingS,
	}
	if result.NextQuestion != nil {
		payload.NextQuestion = &answerJobQuestionShape{
			TextEs:         result.NextQuestion.TextEs,
			TextEn:         result.NextQuestion.TextEn,
			Area:           result.NextQuestion.Area,
			Kind:           string(result.NextQuestion.Kind),
			TurnID:         result.NextQuestion.TurnID,
			QuestionNumber: result.NextQuestion.QuestionNumber,
			TotalQuestions: result.NextQuestion.TotalQuestions,
		}
	}
	return payload
}

// SubmitAnswer processes one answer according to the explicit flow step.
func (s *Service) SubmitAnswer(ctx context.Context, sessionCode, answerText, questionText, turnID string) (*AnswerResult, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	sess, err := s.sessionGetter.GetSessionByCode(dbCtx, sessionCode)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	if time.Now().After(sess.ExpiresAt) {
		slog.Debug("submit answer rejected: session expired", "session_code", sessionCode)
		return nil, session.ErrSessionExpired
	}
	flowState, err := s.store.GetFlowState(dbCtx, sessionCode)
	if err != nil {
		return nil, fmt.Errorf("get flow state: %w", err)
	}

	areas, currentArea, err := s.refreshAreaState(dbCtx, sessionCode)
	if err != nil {
		return nil, fmt.Errorf("refresh area state: %w", err)
	}

	if flowState.Step == FlowStepDone {
		s.finishSession(ctx, sessionCode)
		return &AnswerResult{Done: true, TimerRemainingS: 0}, nil
	}
	if strings.TrimSpace(turnID) == "" || turnID != flowState.ExpectedTurnID {
		return nil, ErrTurnConflict
	}

	timeRemainingS := s.calcTimeRemaining(sess)
	if timeRemainingS <= 0 {
		s.markRemainingNotAssessed(ctx, sessionCode, areas)
		if err := s.store.MarkFlowDone(ctx, sessionCode); err != nil {
			slog.Warn("failed to mark flow done on timeout", "session", sessionCode, "error", err)
		}
		s.finishSession(ctx, sessionCode)
		return &AnswerResult{Done: true, TimerRemainingS: 0}, nil
	}

	preferredLanguage := normalizePreferredLanguage(sess.PreferredLanguage)

	switch flowState.Step {
	case FlowStepDisclaimer:
		if currentArea == nil {
			s.finishSession(ctx, sessionCode)
			return &AnswerResult{Done: true, TimerRemainingS: 0}, nil
		}

		nextTurnID, err := newTurnID()
		if err != nil {
			return nil, fmt.Errorf("new turn id: %w", err)
		}
		nextFlow, err := s.store.AdvanceNonCriterionStep(dbCtx, AdvanceNonCriterionStepParams{
			SessionCode:    sessionCode,
			ExpectedTurnID: turnID,
			CurrentStep:    FlowStepDisclaimer,
			NextStep:       FlowStepReadiness,
			NextTurnID:     nextTurnID,
			EventType:      "disclaimer_ack",
			AnswerText:     answerText,
		})
		if err != nil {
			return nil, fmt.Errorf("advance disclaimer step: %w", err)
		}

		readinessTextEs := s.readinessTextEs
		readinessTextEn := s.readinessTextEn
		// If question number is already beyond the first turn, this disclaimer
		// is part of a resumed interview path. Use explicit resume wording.
		if flowState.QuestionNumber > 1 {
			resumeQuestion := ResumeQuestion(currentArea.Area)
			readinessTextEs = resumeQuestion.TextEs
			readinessTextEn = resumeQuestion.TextEn
		}

		return &AnswerResult{
			Done: false,
			NextQuestion: ReadinessQuestion(
				currentArea.Area,
				readinessTextEs,
				readinessTextEn,
				nextFlow.QuestionNumber,
				nextTurnID,
			),
			TimerRemainingS: timeRemainingS,
		}, nil

	case FlowStepReadiness:
		if currentArea == nil {
			s.finishSession(ctx, sessionCode)
			return &AnswerResult{Done: true, TimerRemainingS: 0}, nil
		}

		nextTurnID, err := newTurnID()
		if err != nil {
			return nil, fmt.Errorf("new turn id: %w", err)
		}
		nextFlow, err := s.store.AdvanceNonCriterionStep(dbCtx, AdvanceNonCriterionStepParams{
			SessionCode:    sessionCode,
			ExpectedTurnID: turnID,
			CurrentStep:    FlowStepReadiness,
			NextStep:       FlowStepCriterion,
			NextTurnID:     nextTurnID,
			EventType:      "readiness_ack",
			AnswerText:     answerText,
		})
		if err != nil {
			return nil, fmt.Errorf("advance readiness step: %w", err)
		}

		answers, err := s.store.GetAnswersBySession(dbCtx, sessionCode)
		if err != nil {
			return nil, fmt.Errorf("get answers: %w", err)
		}
		areaCfg, areaIndex := s.findAreaConfig(currentArea.Area)
		criteriaCoverage := s.buildCriteriaCoverage(areas)
		criteriaRemaining := s.countCriteriaRemaining(areas)
		transcript := s.buildTranscript(answers)

		nextQuestion := s.fallbackQuestionForArea(currentArea.Area)
		turnCtx := &AITurnContext{
			PreferredLanguage:  preferredLanguage,
			CurrentAreaSlug:    currentArea.Area,
			CurrentAreaID:      areaCfg.ID,
			CurrentAreaIndex:   areaIndex,
			IsOpeningTurn:      true,
			CurrentAreaLabel:   areaCfg.Label,
			Description:        areaCfg.Description,
			SufficiencyReqs:    areaCfg.SufficiencyRequirements,
			AreaStatus:         string(currentArea.Status),
			IsPreAddressed:     currentArea.Status == AreaStatusPreAddressed,
			FollowUpsRemaining: MaxQuestionsPerArea - currentArea.QuestionsCount,
			TotalBudgetS:       sess.InterviewBudgetSeconds,
			TimeRemainingS:     timeRemainingS,
			QuestionsRemaining: EstimatedTotalQuestions - len(answers),
			CriteriaRemaining:  criteriaRemaining,
			CriteriaCoverage:   criteriaCoverage,
			Transcript:         transcript,
		}

		slog.Debug("calling AI for first criterion question", "session", sessionCode, "area", currentArea.Area)
		aiResult, err := s.aiClient.CallAI(ctx, turnCtx)
		if err != nil {
			slog.Warn("AI API error on first criterion question, using fallback", "error", err, "area", currentArea.Area)
		} else if candidate := strings.TrimSpace(aiResult.NextQuestion); candidate != "" {
			nextQuestion = candidate
		} else {
			slog.Warn("AI returned empty first criterion question, using fallback", "session", sessionCode, "area", currentArea.Area)
		}

		return &AnswerResult{
			Done: false,
			NextQuestion: &Question{
				TextEs:         nextQuestion,
				TextEn:         nextQuestion,
				Area:           currentArea.Area,
				Kind:           QuestionKindCriterion,
				TurnID:         nextTurnID,
				QuestionNumber: nextFlow.QuestionNumber,
				TotalQuestions: EstimatedTotalQuestions,
			},
			TimerRemainingS: timeRemainingS,
		}, nil

	case FlowStepCriterion:
		if currentArea == nil {
			if err := s.store.MarkFlowDone(ctx, sessionCode); err != nil {
				slog.Warn("failed to mark flow done with no in-progress area", "session", sessionCode, "error", err)
			}
			s.finishSession(ctx, sessionCode)
			return &AnswerResult{Done: true, TimerRemainingS: 0}, nil
		}

		answers, err := s.store.GetAnswersBySession(dbCtx, sessionCode)
		if err != nil {
			return nil, fmt.Errorf("get answers: %w", err)
		}

		areaCfg, areaIndex := s.findAreaConfig(currentArea.Area)
		criteriaCoverage := s.buildCriteriaCoverage(areas)
		transcript := s.buildTranscript(answers)
		criteriaRemaining := s.countCriteriaRemaining(areas)

		turnCtx := &AITurnContext{
			PreferredLanguage:  preferredLanguage,
			CurrentAreaSlug:    currentArea.Area,
			CurrentAreaID:      areaCfg.ID,
			CurrentAreaIndex:   areaIndex,
			IsOpeningTurn:      false,
			CurrentAreaLabel:   areaCfg.Label,
			Description:        areaCfg.Description,
			SufficiencyReqs:    areaCfg.SufficiencyRequirements,
			AreaStatus:         string(currentArea.Status),
			IsPreAddressed:     currentArea.Status == AreaStatusPreAddressed,
			FollowUpsRemaining: MaxQuestionsPerArea - currentArea.QuestionsCount,
			TotalBudgetS:       sess.InterviewBudgetSeconds,
			TimeRemainingS:     timeRemainingS,
			QuestionsRemaining: EstimatedTotalQuestions - len(answers),
			CriteriaRemaining:  criteriaRemaining,
			CriteriaCoverage:   criteriaCoverage,
			Transcript:         transcript,
		}

		slog.Debug("calling AI for criterion turn", "session", sessionCode, "area", currentArea.Area)
		aiResult, err := s.aiClient.CallAI(ctx, turnCtx)
		if err != nil {
			slog.Warn("AI API error, using fallback evaluation", "error", err, "area", currentArea.Area)
			aiResult = &AIResponse{
				Evaluation:   s.fallbackEvaluation(areaCfg.ID),
				NextQuestion: s.fallbackQuestionForArea(currentArea.Area),
			}
		}

		if aiResult.Evaluation == nil || aiResult.Evaluation.CurrentCriterion.ID != areaCfg.ID {
			if aiResult.Evaluation != nil {
				slog.Warn("AI evaluation criterion mismatch, replacing with fallback",
					"session", sessionCode,
					"current_area", currentArea.Area,
					"expected_criterion_id", areaCfg.ID,
					"returned_criterion_id", aiResult.Evaluation.CurrentCriterion.ID,
				)
			}
			aiResult.Evaluation = s.fallbackEvaluation(areaCfg.ID)
		}

		nextTurnID, err := newTurnID()
		if err != nil {
			return nil, fmt.Errorf("new turn id: %w", err)
		}

		preAddressed := s.extractPreAddressed(aiResult.Evaluation.OtherCriteriaAddressed)
		processCtx, processCancel := context.WithTimeout(ctx, dbTimeout)
		result, err := s.store.ProcessCriterionTurn(processCtx, ProcessCriterionTurnParams{
			SessionCode:         sessionCode,
			ExpectedTurnID:      turnID,
			CurrentArea:         currentArea.Area,
			QuestionText:        questionText,
			AnswerText:          answerText,
			PreferredLanguage:   preferredLanguage,
			Evaluation:          aiResult.Evaluation,
			PreAddressed:        preAddressed,
			OrderedAreaSlugs:    s.orderedAreaSlugs(),
			MaxQuestionsPerArea: MaxQuestionsPerArea,
			NextTurnID:          nextTurnID,
		})
		processCancel()
		if err != nil {
			if errors.Is(err, ErrTurnConflict) {
				return nil, ErrTurnConflict
			}
			return nil, fmt.Errorf("process criterion turn: %w", err)
		}

		refreshCtx, refreshCancel := context.WithTimeout(ctx, dbTimeout)
		areas, _, err = s.refreshAreaState(refreshCtx, sessionCode)
		refreshCancel()
		if err != nil {
			return nil, fmt.Errorf("refresh areas after criterion: %w", err)
		}

		timeRemainingS = s.calcTimeRemaining(sess)
		if timeRemainingS <= 0 {
			s.markRemainingNotAssessed(ctx, sessionCode, areas)
			if err := s.store.MarkFlowDone(ctx, sessionCode); err != nil {
				slog.Warn("failed to mark flow done on timeout after criterion", "session", sessionCode, "error", err)
			}
			s.finishSession(ctx, sessionCode)
			return &AnswerResult{Done: true, TimerRemainingS: 0}, nil
		}

		if strings.TrimSpace(result.NextArea) == "" {
			if err := s.store.MarkFlowDone(ctx, sessionCode); err != nil {
				slog.Warn("failed to mark flow done on final criterion", "session", sessionCode, "error", err)
			}
			s.finishSession(ctx, sessionCode)
			return &AnswerResult{Done: true, TimerRemainingS: 0}, nil
		}

		nextQuestion := strings.TrimSpace(aiResult.NextQuestion)
		if result.Action == "next" {
			nextQuestion = s.fallbackQuestionForArea(result.NextArea)

			var nextAreaState *QuestionArea
			for i := range areas {
				if areas[i].Area == result.NextArea {
					nextAreaState = &areas[i]
					break
				}
			}
			if nextAreaState != nil {
				answersCtx, answersCancel := context.WithTimeout(ctx, dbTimeout)
				latestAnswers, err := s.store.GetAnswersBySession(answersCtx, sessionCode)
				answersCancel()
				if err != nil {
					slog.Warn("failed to load answers for next-area opening question", "session", sessionCode, "area", result.NextArea, "error", err)
				} else {
					nextAreaCfg, nextAreaIndex := s.findAreaConfig(result.NextArea)
					nextAreaCoverage := s.buildCriteriaCoverage(areas)
					nextAreaRemaining := s.countCriteriaRemaining(areas)
					nextAreaTranscript := s.buildTranscript(latestAnswers)

					openingTurnCtx := &AITurnContext{
						PreferredLanguage:  preferredLanguage,
						CurrentAreaSlug:    result.NextArea,
						CurrentAreaID:      nextAreaCfg.ID,
						CurrentAreaIndex:   nextAreaIndex,
						IsOpeningTurn:      true,
						CurrentAreaLabel:   nextAreaCfg.Label,
						Description:        nextAreaCfg.Description,
						SufficiencyReqs:    nextAreaCfg.SufficiencyRequirements,
						AreaStatus:         string(nextAreaState.Status),
						IsPreAddressed:     nextAreaState.Status == AreaStatusPreAddressed,
						FollowUpsRemaining: MaxQuestionsPerArea - nextAreaState.QuestionsCount,
						TotalBudgetS:       sess.InterviewBudgetSeconds,
						TimeRemainingS:     timeRemainingS,
						QuestionsRemaining: EstimatedTotalQuestions - len(latestAnswers),
						CriteriaRemaining:  nextAreaRemaining,
						CriteriaCoverage:   nextAreaCoverage,
						Transcript:         nextAreaTranscript,
					}

					slog.Debug("calling AI for next criterion opening question", "session", sessionCode, "area", result.NextArea)
					nextAreaAIResult, err := s.aiClient.CallAI(ctx, openingTurnCtx)
					if err != nil {
						slog.Warn("AI API error on next criterion opening question, using fallback", "error", err, "area", result.NextArea)
					} else if candidate := strings.TrimSpace(nextAreaAIResult.NextQuestion); candidate != "" {
						nextQuestion = candidate
					} else {
						slog.Warn("AI returned empty next criterion opening question, using fallback", "session", sessionCode, "area", result.NextArea)
					}
				}
			}
		}
		if nextQuestion == "" {
			slog.Warn("next question is empty after AI processing, using fallback", "session", sessionCode, "area", result.NextArea)
			nextQuestion = s.fallbackQuestionForArea(result.NextArea)
		}

		return &AnswerResult{
			Done: false,
			NextQuestion: &Question{
				TextEs:         nextQuestion,
				TextEn:         nextQuestion,
				Area:           result.NextArea,
				Kind:           QuestionKindCriterion,
				TurnID:         nextTurnID,
				QuestionNumber: result.QuestionNumber,
				TotalQuestions: EstimatedTotalQuestions,
			},
			TimerRemainingS: timeRemainingS,
		}, nil
	default:
		return nil, ErrInvalidFlow
	}
}

// finishSession marks the session as completed. Logs on error but does not
// propagate — the interview result has already been determined.
func (s *Service) finishSession(ctx context.Context, sessionCode string) {
	if err := s.sessionCompleter.CompleteSession(ctx, sessionCode); err != nil {
		slog.Error("failed to complete session", "session", sessionCode, "error", err)
	}
}

func (s *Service) orderedAreaSlugs() []string {
	slugs := make([]string, 0, len(s.areaConfigs))
	for _, cfg := range s.areaConfigs {
		slugs = append(slugs, cfg.Slug)
	}
	return slugs
}

func (s *Service) fallbackQuestionForArea(slug string) string {
	areaCfg, _ := s.findAreaConfig(slug)
	nextQuestion := strings.TrimSpace(areaCfg.FallbackQuestion)
	if nextQuestion == "" {
		nextQuestion = fmt.Sprintf("Please tell me about %s.", areaCfg.Label)
	}
	return nextQuestion
}

func (s *Service) fallbackEvaluation(criterionID int) *Evaluation {
	return &Evaluation{
		CurrentCriterion: CurrentCriterion{
			ID:              criterionID,
			Status:          "partially_sufficient",
			EvidenceSummary: "Fallback evaluation due to model parsing or provider error.",
			Recommendation:  "follow_up",
		},
		OtherCriteriaAddressed: nil,
	}
}

func (s *Service) extractPreAddressed(other []OtherCriterion) []PreAddressedArea {
	flags := make([]PreAddressedArea, 0, len(other))
	for _, item := range other {
		slug := s.matchAreaSlug(item.Name)
		if slug == "" {
			slog.Warn("cross-criteria flag: no matching area", "name", item.Name)
			continue
		}
		flags = append(flags, PreAddressedArea{
			Slug:     slug,
			Evidence: item.EvidenceSummary,
		})
	}
	return flags
}

// ── Helper methods ──────────────────────────────────────────────────

func (s *Service) calcTimeRemaining(sess *session.Session) int {
	if sess.CurrentInterviewStartedAt == nil {
		return sess.InterviewBudgetSeconds - sess.InterviewLapsedSeconds
	}
	elapsed := int(time.Since(*sess.CurrentInterviewStartedAt).Seconds())
	remaining := sess.InterviewBudgetSeconds - sess.InterviewLapsedSeconds - elapsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (s *Service) findAreaConfig(slug string) (config.AreaConfig, int) {
	for i, ac := range s.areaConfigs {
		if ac.Slug == slug {
			return ac, i
		}
	}
	// Return a minimal config if not found (shouldn't happen in practice).
	return config.AreaConfig{Slug: slug, Label: slug}, -1
}

func (s *Service) buildCriteriaCoverage(areas []QuestionArea) []CriteriaCoverage {
	coverage := make([]CriteriaCoverage, 0, len(areas))
	for _, a := range areas {
		cfg, _ := s.findAreaConfig(a.Area)
		coverage = append(coverage, CriteriaCoverage{
			ID:     cfg.ID,
			Name:   a.Area,
			Status: string(a.Status),
		})
	}
	return coverage
}

func (s *Service) buildTranscript(answers []Answer) []TranscriptEntry {
	transcript := make([]TranscriptEntry, 0, len(answers))
	for i, a := range answers {
		answerText := a.TranscriptEs
		if a.TranscriptEn != "" {
			answerText = a.TranscriptEn
		}
		transcript = append(transcript, TranscriptEntry{
			QuestionNumber: i + 1,
			Criterion:      a.Area,
			Question:       a.QuestionText,
			Answer:         answerText,
		})
	}
	return transcript
}

func (s *Service) countCriteriaRemaining(areas []QuestionArea) int {
	count := 0
	for _, a := range areas {
		if a.Status != AreaStatusComplete && a.Status != AreaStatusInsufficient && a.Status != AreaStatusNotAssessed {
			count++
		}
	}
	return count
}

// matchAreaSlug tries to find a matching area slug from the AI's cross-criteria name.
// Uses case-insensitive matching against both slugs and labels.
func (s *Service) matchAreaSlug(name string) string {
	lower := strings.ToLower(name)
	for _, ac := range s.areaConfigs {
		if strings.ToLower(ac.Slug) == lower || strings.ToLower(ac.Label) == lower {
			return ac.Slug
		}
	}
	return ""
}

func (s *Service) markRemainingNotAssessed(ctx context.Context, sessionCode string, areas []QuestionArea) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()
	for _, a := range areas {
		if a.Status == AreaStatusPending || a.Status == AreaStatusPreAddressed || a.Status == AreaStatusInProgress {
			if err := s.store.MarkAreaNotAssessed(dbCtx, sessionCode, a.Area); err != nil {
				slog.Warn("failed to mark not_assessed", "area", a.Area, "error", err)
			}
		}
	}
}

func (s *Service) refreshAreaState(ctx context.Context, sessionCode string) ([]QuestionArea, *QuestionArea, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	areas, err := s.store.GetAreasBySession(dbCtx, sessionCode)
	if err != nil {
		return nil, nil, fmt.Errorf("get areas by session: %w", err)
	}

	currentArea, err := s.store.GetInProgressArea(dbCtx, sessionCode)
	if err != nil {
		return nil, nil, fmt.Errorf("get in-progress area: %w", err)
	}

	return areas, currentArea, nil
}

func newTurnID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

func normalizePreferredLanguage(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "en":
		return "en"
	default:
		return "es"
	}
}
