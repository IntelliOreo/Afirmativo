// Service layer for interview operations.
// StartInterview: sets session to interviewing, creates areas, returns first question.
// processTurn: persists answer, evaluates via AI, manages area transitions.
package interview

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	areaConfigs      []config.AreaConfig
	openingTextEn    string
	openingTextEs    string
	readinessTextEn  string
	readinessTextEs  string
	nowFn            func() time.Time

	asyncAnswerWorkers       int
	asyncAnswerRecoveryBatch int
	asyncAnswerRecoveryEvery time.Duration
	asyncAnswerStaleAfter    time.Duration
	asyncAnswerJobTimeout    time.Duration
	asyncAnswerQueue         chan string
	asyncRuntimeStartOnce    sync.Once
}

// NewService creates a Service with the given dependencies.
func NewService(
	ss SessionStarter,
	sg SessionGetter,
	sc SessionCompleter,
	store Store,
	ai InterviewAIClient,
	areaConfigs []config.AreaConfig,
	openingTextEn, openingTextEs, readinessTextEn, readinessTextEs string,
	asyncConfig AsyncConfig,
) *Service {
	asyncConfig = asyncConfig.withDefaults()

	svc := &Service{
		sessionStarter:           ss,
		sessionGetter:            sg,
		sessionCompleter:         sc,
		stateStore:               store,
		jobStore:                 store,
		aiClient:                 ai,
		areaConfigs:              areaConfigs,
		openingTextEn:            openingTextEn,
		openingTextEs:            openingTextEs,
		readinessTextEn:          readinessTextEn,
		readinessTextEs:          readinessTextEs,
		nowFn:                    time.Now,
		asyncAnswerWorkers:       asyncConfig.Workers,
		asyncAnswerRecoveryBatch: asyncConfig.RecoveryBatch,
		asyncAnswerRecoveryEvery: asyncConfig.RecoveryEvery,
		asyncAnswerStaleAfter:    asyncConfig.StaleAfter,
		asyncAnswerJobTimeout:    asyncConfig.JobTimeout,
	}
	svc.asyncAnswerQueue = make(chan string, asyncConfig.QueueSize)
	return svc
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

// StartInterview transitions the session to interviewing,
// creates all question area rows, and returns the opening question.
func (s *Service) StartInterview(ctx context.Context, sessionCode, preferredLanguage string) (*StartResult, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	existing, err := s.sessionGetter.GetSessionByCode(dbCtx, sessionCode)
	if err != nil {
		return nil, err
	}
	if s.nowFn().After(existing.ExpiresAt) {
		slog.Debug("start interview rejected: session expired", "session_code", sessionCode)
		return nil, session.ErrSessionExpired
	}

	sess, err := s.sessionStarter.StartSession(dbCtx, sessionCode, preferredLanguage)
	if err != nil {
		return nil, err
	}
	effectiveLanguage := normalizePreferredLanguage(sess.PreferredLanguage)

	for {
		flowCtx, flowCancel := context.WithTimeout(ctx, dbTimeout)
		answersCount, err := s.stateStore.GetAnswerCount(flowCtx, sessionCode)
		if err != nil {
			flowCancel()
			return nil, fmt.Errorf("get answer count: %w", err)
		}
		currentFlow, err := s.stateStore.GetFlowState(flowCtx, sessionCode)
		flowCancel()
		if err != nil {
			return nil, fmt.Errorf("get flow state: %w", err)
		}

		resuming := answersCount > 0 || currentFlow.Step != FlowStepDisclaimer
		if currentFlow.ActiveQuestion != nil {
			if currentFlow.ActiveQuestion.BufferExpired(s.nowFn()) {
				questionText := currentFlow.ActiveQuestion.TextForLanguage(effectiveLanguage)
				if _, err := s.processTurn(ctx, sessionCode, "", questionText, currentFlow.ExpectedTurnID); err != nil {
					return nil, fmt.Errorf("finalize expired active turn: %w", err)
				}
				continue
			}

			overallRemaining := s.calcTimeRemaining(sess)
			if overallRemaining < 0 {
				overallRemaining = 0
			}
			return &StartResult{
				Question:                     &currentFlow.ActiveQuestion.Question,
				TimerRemainingS:              overallRemaining,
				AnswerSubmitWindowRemainingS: currentFlow.ActiveQuestion.SubmitWindowRemaining(s.nowFn()),
				Area:                         currentFlow.ActiveQuestion.Question.Area,
				Resuming:                     resuming,
				Language:                     effectiveLanguage,
			}, nil
		}

		// Pre-create all question area rows (idempotent — ON CONFLICT DO NOTHING).
		for _, area := range s.areaConfigs {
			createCtx, createCancel := context.WithTimeout(ctx, dbTimeout)
			_, err := s.stateStore.CreateQuestionArea(createCtx, sessionCode, area.Slug)
			createCancel()
			if err != nil {
				return nil, fmt.Errorf("create question area %s: %w", area.Slug, err)
			}
		}

		firstArea := s.areaConfigs[0].Slug
		setAreaCtx, setAreaCancel := context.WithTimeout(ctx, dbTimeout)
		if err := s.stateStore.SetAreaInProgress(setAreaCtx, sessionCode, firstArea); err != nil {
			setAreaCancel()
			return nil, fmt.Errorf("set first area in_progress: %w", err)
		}
		setAreaCancel()

		activeArea := firstArea
		activeAreaCtx, activeAreaCancel := context.WithTimeout(ctx, dbTimeout)
		inProgressArea, err := s.stateStore.GetInProgressArea(activeAreaCtx, sessionCode)
		activeAreaCancel()
		if err != nil {
			return nil, fmt.Errorf("get in-progress area: %w", err)
		}
		if inProgressArea != nil && strings.TrimSpace(inProgressArea.Area) != "" {
			activeArea = inProgressArea.Area
		}

		turnID, err := newTurnID()
		if err != nil {
			return nil, fmt.Errorf("new turn id: %w", err)
		}

		var q *Question
		if resuming {
			resumeQuestion := ResumeQuestion(activeArea)
			q = ReadinessQuestion(
				activeArea,
				resumeQuestion.TextEs,
				resumeQuestion.TextEn,
				currentFlow.QuestionNumber,
				turnID,
			)
			persistCtx, persistCancel := context.WithTimeout(ctx, dbTimeout)
			_, err = s.stateStore.PrepareReadinessStep(persistCtx, sessionCode, NewIssuedQuestion(q, s.nowFn()))
			persistCancel()
			if err != nil {
				return nil, fmt.Errorf("prepare readiness step: %w", err)
			}
		} else {
			q = OpeningDisclaimerQuestion(
				activeArea,
				s.openingTextEs,
				s.openingTextEn,
				currentFlow.QuestionNumber,
				turnID,
			)
			persistCtx, persistCancel := context.WithTimeout(ctx, dbTimeout)
			_, err = s.stateStore.PrepareDisclaimerStep(persistCtx, sessionCode, NewIssuedQuestion(q, s.nowFn()))
			persistCancel()
			if err != nil {
				return nil, fmt.Errorf("prepare disclaimer step: %w", err)
			}
		}

		overallRemaining := s.calcTimeRemaining(sess)
		if overallRemaining < 0 {
			overallRemaining = 0
		}
		return &StartResult{
			Question:                     q,
			TimerRemainingS:              overallRemaining,
			AnswerSubmitWindowRemainingS: int(AnswerSubmitWindow / time.Second),
			Area:                         activeArea,
			Resuming:                     resuming,
			Language:                     effectiveLanguage,
		}, nil
	}
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
	return s.processTurnCore(ctx, sessionCode, answerText, questionText, turnID, nil)
}

func (s *Service) processTurnForAsyncJob(ctx context.Context, job *AnswerJob) (*AnswerResult, error) {
	return s.processTurnCore(
		ctx,
		job.SessionCode,
		job.AnswerText,
		job.QuestionText,
		job.TurnID,
		s.newAsyncJobRetryFailureRecorder(job.ID),
	)
}

func (s *Service) processTurnCore(
	ctx context.Context,
	sessionCode, answerText, questionText, turnID string,
	failureRecorder aiRetryFailureRecorder,
) (*AnswerResult, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	sess, err := s.sessionGetter.GetSessionByCode(dbCtx, sessionCode)
	dbCancel()
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	if s.nowFn().After(sess.ExpiresAt) {
		slog.Debug("submit answer rejected: session expired", "session_code", sessionCode)
		return nil, session.ErrSessionExpired
	}

	dbCtx, dbCancel = context.WithTimeout(ctx, dbTimeout)
	flowState, err := s.stateStore.GetFlowState(dbCtx, sessionCode)
	dbCancel()
	if err != nil {
		return nil, fmt.Errorf("get flow state: %w", err)
	}

	areas, currentArea, err := s.refreshAreaState(ctx, sessionCode)
	if err != nil {
		return nil, fmt.Errorf("refresh area state: %w", err)
	}

	if flowState.Step == FlowStepDone {
		s.finishSession(ctx, sessionCode)
		return &AnswerResult{Done: true, TimerRemainingS: 0, AnswerSubmitWindowRemainingS: 0}, nil
	}
	if strings.TrimSpace(turnID) == "" || turnID != flowState.ExpectedTurnID {
		return nil, ErrTurnConflict
	}

	timeRemainingS := s.calcTimeRemaining(sess)
	if timeRemainingS <= 0 {
		return s.finishOnTimeout(ctx, sessionCode, areas)
	}

	preferredLanguage := normalizePreferredLanguage(sess.PreferredLanguage)

	switch flowState.Step {
	case FlowStepDisclaimer:
		if s.finishIfNoCurrentArea(ctx, sessionCode, currentArea, false) {
			return &AnswerResult{Done: true, TimerRemainingS: 0, AnswerSubmitWindowRemainingS: 0}, nil
		}

		readinessTextEs := s.readinessTextEs
		readinessTextEn := s.readinessTextEn
		if flowState.QuestionNumber > 1 {
			resumeQuestion := ResumeQuestion(currentArea.Area)
			readinessTextEs = resumeQuestion.TextEs
			readinessTextEn = resumeQuestion.TextEn
		}

		nextTurnID, err := newTurnID()
		if err != nil {
			return nil, fmt.Errorf("new turn id: %w", err)
		}
		nextQuestion := ReadinessQuestion(
			currentArea.Area,
			readinessTextEs,
			readinessTextEn,
			flowState.QuestionNumber,
			nextTurnID,
		)
		issuedQuestion := NewIssuedQuestion(nextQuestion, s.nowFn())

		advanceCtx, advanceCancel := context.WithTimeout(ctx, dbTimeout)
		nextFlow, err := s.stateStore.AdvanceNonCriterionStep(advanceCtx, AdvanceNonCriterionStepParams{
			SessionCode:        sessionCode,
			ExpectedTurnID:     turnID,
			CurrentStep:        FlowStepDisclaimer,
			NextStep:           FlowStepReadiness,
			EventType:          "disclaimer_ack",
			AnswerText:         answerText,
			NextIssuedQuestion: issuedQuestion,
		})
		advanceCancel()
		if err != nil {
			return nil, fmt.Errorf("advance disclaimer step: %w", err)
		}

		return &AnswerResult{
			Done: false,
			NextQuestion: func() *Question {
				if nextFlow.ActiveQuestion != nil {
					return &nextFlow.ActiveQuestion.Question
				}
				return nextQuestion
			}(),
			TimerRemainingS:              timeRemainingS,
			AnswerSubmitWindowRemainingS: issuedQuestion.SubmitWindowRemaining(s.nowFn()),
		}, nil

	case FlowStepReadiness:
		if s.finishIfNoCurrentArea(ctx, sessionCode, currentArea, false) {
			return &AnswerResult{Done: true, TimerRemainingS: 0, AnswerSubmitWindowRemainingS: 0}, nil
		}

		answersCtx, answersCancel := context.WithTimeout(ctx, dbTimeout)
		answers, err := s.stateStore.GetAnswersBySession(answersCtx, sessionCode)
		answersCancel()
		if err != nil {
			return nil, fmt.Errorf("get answers: %w", err)
		}
		areaCfg, areaIndex := s.findAreaConfig(currentArea.Area)
		criteriaCoverage := s.buildCriteriaCoverage(areas)
		criteriaRemaining := s.countCriteriaRemaining(areas)
		historyTurns := s.buildHistoryTurns(answers, preferredLanguage)

		nextQuestionText := s.fallbackQuestionForArea(currentArea.Area)
		turnCtx := &AITurnContext{
			PreferredLanguage:  preferredLanguage,
			CurrentAreaSlug:    currentArea.Area,
			CurrentAreaID:      areaCfg.ID,
			CurrentAreaIndex:   areaIndex,
			IsOpeningTurn:      true,
			CurrentAreaLabel:   areaCfg.Label,
			Description:        areaCfg.Description,
			SufficiencyReqs:    areaCfg.SufficiencyRequirements,
			AreaStatus:         currentArea.Status,
			IsPreAddressed:     currentArea.Status == AreaStatusPreAddressed,
			FollowUpsRemaining: MaxQuestionsPerArea - currentArea.QuestionsCount,
			TotalBudgetS:       sess.InterviewBudgetSeconds,
			TimeRemainingS:     timeRemainingS,
			QuestionsRemaining: EstimatedTotalQuestions - len(answers),
			CriteriaRemaining:  criteriaRemaining,
			CriteriaCoverage:   criteriaCoverage,
			HistoryTurns:       historyTurns,
		}

		slog.Debug("calling AI for first criterion question", "session", sessionCode, "area", currentArea.Area)
		substituted := false
		aiResult, err := s.callAIWithRetry(ctx, turnCtx, failureRecorder)
		if err != nil {
			if !errors.Is(err, ErrAIRetryExhausted) {
				return nil, err
			}
			substituted = true
			slog.Warn("AI retries exhausted on first criterion question, using fallback", "error", err, "area", currentArea.Area)
		} else if candidate := strings.TrimSpace(aiResult.NextQuestion); candidate != "" {
			nextQuestionText = candidate
		} else {
			substituted = true
			slog.Warn("AI returned empty first criterion question, using fallback", "session", sessionCode, "area", currentArea.Area)
		}

		nextTurnID, err := newTurnID()
		if err != nil {
			return nil, fmt.Errorf("new turn id: %w", err)
		}

		nextQuestion := &Question{
			TextEs:         nextQuestionText,
			TextEn:         nextQuestionText,
			Area:           currentArea.Area,
			Kind:           QuestionKindCriterion,
			TurnID:         nextTurnID,
			QuestionNumber: flowState.QuestionNumber + 1,
			TotalQuestions: EstimatedTotalQuestions,
		}
		issuedQuestion := NewIssuedQuestion(nextQuestion, s.nowFn())
		advanceCtx, advanceCancel := context.WithTimeout(ctx, dbTimeout)
		nextFlow, err := s.stateStore.AdvanceNonCriterionStep(advanceCtx, AdvanceNonCriterionStepParams{
			SessionCode:        sessionCode,
			ExpectedTurnID:     turnID,
			CurrentStep:        FlowStepReadiness,
			NextStep:           FlowStepCriterion,
			EventType:          "readiness_ack",
			AnswerText:         answerText,
			NextIssuedQuestion: issuedQuestion,
		})
		advanceCancel()
		if err != nil {
			return nil, fmt.Errorf("advance readiness step: %w", err)
		}

		return &AnswerResult{
			Done: false,
			NextQuestion: func() *Question {
				if nextFlow.ActiveQuestion != nil {
					return &nextFlow.ActiveQuestion.Question
				}
				return nextQuestion
			}(),
			TimerRemainingS:              timeRemainingS,
			AnswerSubmitWindowRemainingS: issuedQuestion.SubmitWindowRemaining(s.nowFn()),
			Substituted:                  substituted,
		}, nil

	case FlowStepCriterion:
		if s.finishIfNoCurrentArea(ctx, sessionCode, currentArea, true) {
			return &AnswerResult{Done: true, TimerRemainingS: 0, AnswerSubmitWindowRemainingS: 0}, nil
		}

		answersCtx, answersCancel := context.WithTimeout(ctx, dbTimeout)
		answers, err := s.stateStore.GetAnswersBySession(answersCtx, sessionCode)
		answersCancel()
		if err != nil {
			return nil, fmt.Errorf("get answers: %w", err)
		}

		areaCfg, areaIndex := s.findAreaConfig(currentArea.Area)
		criteriaCoverage := s.buildCriteriaCoverage(areas)
		historyTurns := s.buildHistoryTurns(answers, preferredLanguage)
		criteriaRemaining := s.countCriteriaRemaining(areas)

		turnCtx := &AITurnContext{
			PreferredLanguage:   preferredLanguage,
			CurrentAreaSlug:     currentArea.Area,
			CurrentAreaID:       areaCfg.ID,
			CurrentAreaIndex:    areaIndex,
			IsOpeningTurn:       false,
			CurrentQuestionText: questionText,
			LatestAnswerText:    answerText,
			CurrentAreaLabel:    areaCfg.Label,
			Description:         areaCfg.Description,
			SufficiencyReqs:     areaCfg.SufficiencyRequirements,
			AreaStatus:          currentArea.Status,
			IsPreAddressed:      currentArea.Status == AreaStatusPreAddressed,
			FollowUpsRemaining:  MaxQuestionsPerArea - currentArea.QuestionsCount,
			TotalBudgetS:        sess.InterviewBudgetSeconds,
			TimeRemainingS:      timeRemainingS,
			QuestionsRemaining:  EstimatedTotalQuestions - len(answers),
			CriteriaRemaining:   criteriaRemaining,
			CriteriaCoverage:    criteriaCoverage,
			HistoryTurns:        historyTurns,
		}

		slog.Debug("calling AI for criterion turn", "session", sessionCode, "area", currentArea.Area)
		substituted := false
		aiResult, err := s.callAIWithRetry(ctx, turnCtx, failureRecorder)
		if err != nil {
			if !errors.Is(err, ErrAIRetryExhausted) {
				return nil, err
			}
			substituted = true
			slog.Warn("AI retries exhausted, using fallback evaluation", "error", err, "area", currentArea.Area)
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
			substituted = true
		}

		preAddressed := s.extractPreAddressed(aiResult.Evaluation.OtherCriteriaAddressed)
		decision := DecideCriterionTurn(
			aiResult.Evaluation.CurrentCriterion,
			currentArea.QuestionsCount+1,
			MaxQuestionsPerArea,
		)
		projectedAreas := s.projectAreasForNextAreaOpening(areas, currentArea.Area, decision, preAddressed)
		projectedAnswers := buildAnswersWithCurrentTurn(answers, questionText, answerText, preferredLanguage)
		nextArea := DetermineNextAreaAfterCriterionTurn(
			projectedAreas,
			currentArea.Area,
			decision,
			preAddressed,
			s.orderedAreaSlugs(),
		)

		timeRemainingS = s.calcTimeRemaining(sess)
		if timeRemainingS <= 0 {
			return s.finishOnTimeout(ctx, sessionCode, areas)
		}

		var nextQuestion *Question
		var issuedQuestion *IssuedQuestion
		if strings.TrimSpace(nextArea) != "" {
			nextTurnID, err := newTurnID()
			if err != nil {
				return nil, fmt.Errorf("new turn id: %w", err)
			}

			nextQuestionText := strings.TrimSpace(aiResult.NextQuestion)
			if decision.Action == CriterionTurnActionNext {
				var nextAreaSubstituted bool
				nextQuestionText, nextAreaSubstituted, err = s.generateNextAreaOpeningQuestion(
					ctx,
					sessionCode,
					nextArea,
					projectedAreas,
					projectedAnswers,
					sess,
					preferredLanguage,
					timeRemainingS,
					failureRecorder,
				)
				if err != nil {
					return nil, err
				}
				substituted = substituted || nextAreaSubstituted
			}
			if nextQuestionText == "" {
				substituted = true
				slog.Warn("next question is empty after AI processing, using fallback", "session", sessionCode, "area", nextArea)
				nextQuestionText = s.fallbackQuestionForArea(nextArea)
			}

			nextQuestion = &Question{
				TextEs:         nextQuestionText,
				TextEn:         nextQuestionText,
				Area:           nextArea,
				Kind:           QuestionKindCriterion,
				TurnID:         nextTurnID,
				QuestionNumber: flowState.QuestionNumber + 1,
				TotalQuestions: EstimatedTotalQuestions,
			}
			issuedQuestion = NewIssuedQuestion(nextQuestion, s.nowFn())
		}

		processCtx, processCancel := context.WithTimeout(ctx, dbTimeout)
		_, err = s.stateStore.ProcessCriterionTurn(processCtx, ProcessCriterionTurnParams{
			SessionCode:        sessionCode,
			ExpectedTurnID:     turnID,
			CurrentArea:        currentArea.Area,
			QuestionText:       questionText,
			AnswerText:         answerText,
			PreferredLanguage:  preferredLanguage,
			Evaluation:         aiResult.Evaluation,
			PreAddressed:       preAddressed,
			Decision:           decision,
			NextArea:           nextArea,
			NextIssuedQuestion: issuedQuestion,
		})
		processCancel()
		if err != nil {
			if errors.Is(err, ErrTurnConflict) {
				return nil, ErrTurnConflict
			}
			return nil, fmt.Errorf("process criterion turn: %w", err)
		}

		areas, _, err = s.refreshAreaState(ctx, sessionCode)
		if err != nil {
			return nil, fmt.Errorf("refresh areas after criterion: %w", err)
		}

		timeRemainingS = s.calcTimeRemaining(sess)
		if timeRemainingS <= 0 {
			return s.finishOnTimeout(ctx, sessionCode, areas)
		}

		if strings.TrimSpace(nextArea) == "" {
			s.finishSession(ctx, sessionCode)
			return &AnswerResult{Done: true, TimerRemainingS: 0, AnswerSubmitWindowRemainingS: 0}, nil
		}

		return &AnswerResult{
			Done: false,
			NextQuestion: func() *Question {
				if issuedQuestion != nil {
					return &issuedQuestion.Question
				}
				return nextQuestion
			}(),
			TimerRemainingS:              timeRemainingS,
			AnswerSubmitWindowRemainingS: issuedQuestion.SubmitWindowRemaining(s.nowFn()),
			Substituted:                  substituted,
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

func (s *Service) finishOnTimeout(ctx context.Context, sessionCode string, areas []QuestionArea) (*AnswerResult, error) {
	s.markRemainingNotAssessed(ctx, sessionCode, areas)
	if err := s.stateStore.MarkFlowDone(ctx, sessionCode); err != nil {
		slog.Warn("failed to mark flow done on timeout", "session", sessionCode, "error", err)
	}
	s.finishSession(ctx, sessionCode)
	return &AnswerResult{Done: true, TimerRemainingS: 0, AnswerSubmitWindowRemainingS: 0}, nil
}

func (s *Service) finishIfNoCurrentArea(ctx context.Context, sessionCode string, currentArea *QuestionArea, markDone bool) bool {
	if currentArea != nil {
		return false
	}
	if markDone {
		if err := s.stateStore.MarkFlowDone(ctx, sessionCode); err != nil {
			slog.Warn("failed to mark flow done with no current area", "session", sessionCode, "error", err)
		}
	}
	s.finishSession(ctx, sessionCode)
	return true
}

func (s *Service) generateNextAreaOpeningQuestion(
	ctx context.Context,
	sessionCode string,
	nextAreaSlug string,
	areas []QuestionArea,
	answers []Answer,
	sess *session.Session,
	preferredLanguage string,
	timeRemainingS int,
	failureRecorder aiRetryFailureRecorder,
) (question string, substituted bool, err error) {
	question = s.fallbackQuestionForArea(nextAreaSlug)

	var nextAreaState *QuestionArea
	for i := range areas {
		if areas[i].Area == nextAreaSlug {
			nextAreaState = &areas[i]
			break
		}
	}
	if nextAreaState == nil {
		return question, false, nil
	}

	nextAreaCfg, nextAreaIndex := s.findAreaConfig(nextAreaSlug)
	openingTurnCtx := &AITurnContext{
		PreferredLanguage:  preferredLanguage,
		CurrentAreaSlug:    nextAreaSlug,
		CurrentAreaID:      nextAreaCfg.ID,
		CurrentAreaIndex:   nextAreaIndex,
		IsOpeningTurn:      true,
		CurrentAreaLabel:   nextAreaCfg.Label,
		Description:        nextAreaCfg.Description,
		SufficiencyReqs:    nextAreaCfg.SufficiencyRequirements,
		AreaStatus:         nextAreaState.Status,
		IsPreAddressed:     nextAreaState.Status == AreaStatusPreAddressed,
		FollowUpsRemaining: MaxQuestionsPerArea - nextAreaState.QuestionsCount,
		TotalBudgetS:       sess.InterviewBudgetSeconds,
		TimeRemainingS:     timeRemainingS,
		QuestionsRemaining: EstimatedTotalQuestions - len(answers),
		CriteriaRemaining:  s.countCriteriaRemaining(areas),
		CriteriaCoverage:   s.buildCriteriaCoverage(areas),
		HistoryTurns:       s.buildHistoryTurns(answers, preferredLanguage),
	}

	slog.Debug("calling AI for next criterion opening question", "session", sessionCode, "area", nextAreaSlug)
	nextAreaAIResult, err := s.callAIWithRetry(ctx, openingTurnCtx, failureRecorder)
	if err != nil {
		if !errors.Is(err, ErrAIRetryExhausted) {
			return "", false, err
		}
		slog.Warn("AI retries exhausted on next criterion opening question, using fallback", "error", err, "area", nextAreaSlug)
		return question, true, nil
	}

	if candidate := strings.TrimSpace(nextAreaAIResult.NextQuestion); candidate != "" {
		return candidate, false, nil
	}

	slog.Warn("AI returned empty next criterion opening question, using fallback", "session", sessionCode, "area", nextAreaSlug)
	return question, true, nil
}

func (s *Service) projectAreasForNextAreaOpening(
	areas []QuestionArea,
	currentArea string,
	decision CriterionTurnDecision,
	preAddressed []PreAddressedArea,
) []QuestionArea {
	projected := make([]QuestionArea, len(areas))
	copy(projected, areas)

	preAddressedEvidence := make(map[string]string, len(preAddressed))
	for _, flag := range preAddressed {
		if strings.TrimSpace(flag.Slug) == "" {
			continue
		}
		preAddressedEvidence[flag.Slug] = flag.Evidence
	}

	for i := range projected {
		switch {
		case projected[i].Area == currentArea:
			if decision.MarkCurrentAs != "" {
				projected[i].Status = decision.MarkCurrentAs
			}
			projected[i].QuestionsCount++
		case projected[i].Status == AreaStatusPending:
			if evidence, ok := preAddressedEvidence[projected[i].Area]; ok {
				projected[i].Status = AreaStatusPreAddressed
				projected[i].PreAddressedEvidence = evidence
			}
		}
	}

	return projected
}

func buildAnswersWithCurrentTurn(answers []Answer, questionText, answerText, preferredLanguage string) []Answer {
	history := make([]Answer, 0, len(answers)+1)
	history = append(history, answers...)

	latest := Answer{QuestionText: questionText}
	if strings.EqualFold(strings.TrimSpace(preferredLanguage), "en") {
		latest.TranscriptEn = answerText
	} else {
		latest.TranscriptEs = answerText
	}
	history = append(history, latest)
	return history
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
			Status:          CriterionStatusPartial,
			EvidenceSummary: "Fallback evaluation due to model parsing or provider error.",
			Recommendation:  CriterionRecFollowUp,
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
	elapsed := int(s.nowFn().Sub(*sess.CurrentInterviewStartedAt).Seconds())
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
			Status: a.Status,
		})
	}
	return coverage
}

func (s *Service) buildHistoryTurns(answers []Answer, preferredLanguage string) []HistoryTurn {
	useEnglish := strings.EqualFold(strings.TrimSpace(preferredLanguage), "en")

	historyTurns := make([]HistoryTurn, 0, len(answers))
	for _, a := range answers {
		historyTurns = append(historyTurns, HistoryTurn{
			QuestionText: a.QuestionText,
			AnswerText:   selectTranscript(useEnglish, a.TranscriptEn, a.TranscriptEs),
		})
	}
	return historyTurns
}

func (s *Service) countCriteriaRemaining(areas []QuestionArea) int {
	count := 0
	for _, a := range areas {
		if isAreaUnresolved(a.Status) {
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
		if isAreaUnresolved(a.Status) {
			if err := s.stateStore.MarkAreaNotAssessed(dbCtx, sessionCode, a.Area); err != nil {
				slog.Warn("failed to mark not_assessed", "area", a.Area, "error", err)
			}
		}
	}
}

func selectTranscript(preferEnglish bool, en, es string) string {
	if preferEnglish {
		if candidate := strings.TrimSpace(en); candidate != "" {
			return en
		}
		return es
	}
	if candidate := strings.TrimSpace(es); candidate != "" {
		return es
	}
	return en
}

func (s *Service) refreshAreaState(ctx context.Context, sessionCode string) ([]QuestionArea, *QuestionArea, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	areas, err := s.stateStore.GetAreasBySession(dbCtx, sessionCode)
	if err != nil {
		return nil, nil, fmt.Errorf("get areas by session: %w", err)
	}

	currentArea, err := s.stateStore.GetInProgressArea(dbCtx, sessionCode)
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
