// Service layer for interview operations.
// StartInterview: sets session to interviewing, creates areas, returns first question.
// processTurn: persists answer, evaluates via AI, manages area transitions.
package interview

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/afirmativo/backend/internal/config"
	"github.com/afirmativo/backend/internal/session"
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

// InterviewAIClient calls the AI API to evaluate answers and generate next questions.
type InterviewAIClient interface {
	GenerateTurn(ctx context.Context, turnCtx *AITurnContext) (*AIResponse, error)
}

type interviewStateStore = InterviewStateStore
type asyncAnswerJobStore = AsyncAnswerJobStore

// Service contains interview business logic.
type Service struct {
	sessionStarter   SessionStarter
	sessionGetter    SessionGetter
	sessionCompleter SessionCompleter
	stateStore       interviewStateStore
	jobStore         asyncAnswerJobStore
	aiClient         InterviewAIClient
	settings         Settings
	nowFn            func() time.Time
	dbTimeout        time.Duration

	asyncAnswerWorkers       int
	asyncAnswerRecoveryEvery time.Duration
	asyncAnswerStaleAfter    time.Duration
	asyncAnswerJobTimeout    time.Duration
	asyncAnswerQueue         chan string
	asyncRuntimeStartOnce    sync.Once
	workerWg                 sync.WaitGroup
}

type Deps struct {
	SessionStarter   SessionStarter
	SessionGetter    SessionGetter
	SessionCompleter SessionCompleter
	Store            Store
	AIClient         InterviewAIClient
}

type Settings struct {
	AreaConfigs            []config.AreaConfig
	OpeningDisclaimer      config.BilingualText
	ReadinessQuestion      config.BilingualText
	AnswerTimeLimitSeconds int
	DBTimeout              time.Duration
	AsyncRuntime           config.AsyncRuntimeConfig
}

// NewService creates a Service with the given dependencies.
func NewService(deps Deps, settings Settings) *Service {
	svc := &Service{
		sessionStarter:           deps.SessionStarter,
		sessionGetter:            deps.SessionGetter,
		sessionCompleter:         deps.SessionCompleter,
		stateStore:               deps.Store,
		jobStore:                 deps.Store,
		aiClient:                 deps.AIClient,
		settings:                 settings,
		nowFn:                    time.Now,
		dbTimeout:                settings.DBTimeout,
		asyncAnswerWorkers:       settings.AsyncRuntime.Workers,
		asyncAnswerRecoveryEvery: settings.AsyncRuntime.RecoveryEvery,
		asyncAnswerStaleAfter:    settings.AsyncRuntime.StaleAfter,
		asyncAnswerJobTimeout:    settings.AsyncRuntime.JobTimeout,
	}
	svc.asyncAnswerQueue = make(chan string, settings.AsyncRuntime.QueueSize)
	return svc
}

// HealthStats returns async runtime stats for the health endpoint.
func (s *Service) HealthStats() map[string]any {
	return map[string]any{
		"async_answer_queue_depth":    len(s.asyncAnswerQueue),
		"async_answer_queue_capacity": cap(s.asyncAnswerQueue),
		"async_answer_workers":        s.asyncAnswerWorkers,
	}
}

// StartResult holds the output of a successful interview start.
type StartResult struct {
	Question                     *Question
	TimerRemainingS              int
	AnswerSubmitWindowRemainingS int
	Area                         string
	Resuming                     bool
	Language                     string
}

// AnswerResult holds the output of a submitted answer.
type AnswerResult struct {
	Done                         bool
	NextQuestion                 *Question
	TimerRemainingS              int
	AnswerSubmitWindowRemainingS int
	Substituted                  bool
}

// processTurn processes one answer according to the explicit flow step.
func (s *Service) processTurn(ctx context.Context, sessionCode, answerText, questionText, turnID string) (*AnswerResult, error) {
	return s.processTurnCore(ctx, sessionCode, answerText, questionText, turnID, s.nowFn(), nil)
}

func (s *Service) processTurnForAsyncJob(ctx context.Context, job *AnswerJob) (*AnswerResult, error) {
	return s.processTurnCore(
		ctx,
		job.SessionCode,
		job.AnswerText,
		job.QuestionText,
		job.TurnID,
		job.CreatedAt,
		s.newAsyncJobRetryFailureRecorder(job.ID),
	)
}

func (s *Service) processTurnCore(
	ctx context.Context,
	sessionCode, answerText, questionText, turnID string,
	submissionTime time.Time,
	failureRecorder aiRetryFailureRecorder,
) (*AnswerResult, error) {
	snapshot, err := s.buildTurnSnapshot(
		ctx,
		sessionCode,
		answerText,
		questionText,
		turnID,
		submissionTime,
		failureRecorder,
	)
	if err != nil {
		return nil, err
	}

	if snapshot.flowState.Step == FlowStepDone {
		if err := s.finishSession(ctx, sessionCode); err != nil {
			return nil, err
		}
		return doneAnswerResult(false), nil
	}
	if strings.TrimSpace(snapshot.turnID) == "" || snapshot.turnID != snapshot.flowState.ExpectedTurnID {
		return nil, ErrTurnConflict
	}
	if snapshot.timeRemainingS <= 0 {
		return s.finishOnTimeout(ctx, sessionCode, snapshot.areas)
	}

	switch snapshot.flowState.Step {
	case FlowStepDisclaimer:
		return s.handleDisclaimerTurn(ctx, sessionCode, snapshot)
	case FlowStepReadiness:
		return s.handleReadinessTurn(ctx, sessionCode, snapshot)
	case FlowStepCriterion:
		return s.handleCriterionTurn(ctx, sessionCode, snapshot)
	default:
		return nil, ErrInvalidFlow
	}
}
