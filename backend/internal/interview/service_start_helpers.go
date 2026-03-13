package interview

import (
	"context"
	"fmt"
	"strings"

	"github.com/afirmativo/backend/internal/session"
)

type startInterviewState struct {
	session           *session.Session
	flowState         *FlowState
	answersCount      int
	resuming          bool
	effectiveLanguage string
}

type startIssuePlan struct {
	resuming bool
	issue    questionIssue
}

func (s *Service) loadStartInterviewState(
	ctx context.Context,
	sessionCode, preferredLanguage string,
) (*startInterviewState, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, s.dbTimeout)
	defer dbCancel()

	existing, err := s.sessionGetter.GetSessionByCode(dbCtx, sessionCode)
	if err != nil {
		return nil, err
	}
	if s.nowFn().After(existing.ExpiresAt) {
		return nil, session.ErrSessionExpired
	}

	sess, err := s.sessionStarter.StartSession(dbCtx, sessionCode, preferredLanguage)
	if err != nil {
		return nil, err
	}

	answersCount, flowState, err := s.loadStartFlowState(ctx, sessionCode)
	if err != nil {
		return nil, err
	}

	return &startInterviewState{
		session:           sess,
		flowState:         flowState,
		answersCount:      answersCount,
		resuming:          answersCount > 0 || flowState.Step != FlowStepDisclaimer,
		effectiveLanguage: normalizePreferredLanguage(sess.PreferredLanguage),
	}, nil
}

func (s *Service) loadStartFlowState(ctx context.Context, sessionCode string) (int, *FlowState, error) {
	flowCtx, flowCancel := context.WithTimeout(ctx, s.dbTimeout)
	defer flowCancel()

	answersCount, err := s.stateStore.GetAnswerCount(flowCtx, sessionCode)
	if err != nil {
		return 0, nil, fmt.Errorf("get answer count: %w", err)
	}

	flowState, err := s.stateStore.GetFlowState(flowCtx, sessionCode)
	if err != nil {
		return 0, nil, fmt.Errorf("get flow state: %w", err)
	}

	return answersCount, flowState, nil
}

func (s *Service) ensureStartAreaState(ctx context.Context, sessionCode string) (string, error) {
	for _, area := range s.settings.AreaConfigs {
		createCtx, createCancel := context.WithTimeout(ctx, s.dbTimeout)
		_, err := s.stateStore.CreateQuestionArea(createCtx, sessionCode, area.Slug)
		createCancel()
		if err != nil {
			return "", fmt.Errorf("create question area %s: %w", area.Slug, err)
		}
	}

	firstArea := s.settings.AreaConfigs[0].Slug
	setAreaCtx, setAreaCancel := context.WithTimeout(ctx, s.dbTimeout)
	if err := s.stateStore.SetAreaInProgress(setAreaCtx, sessionCode, firstArea); err != nil {
		setAreaCancel()
		return "", fmt.Errorf("set first area in_progress: %w", err)
	}
	setAreaCancel()

	activeAreaCtx, activeAreaCancel := context.WithTimeout(ctx, s.dbTimeout)
	inProgressArea, err := s.stateStore.GetInProgressArea(activeAreaCtx, sessionCode)
	activeAreaCancel()
	if err != nil {
		return "", fmt.Errorf("get in-progress area: %w", err)
	}
	if inProgressArea != nil && strings.TrimSpace(inProgressArea.Area) != "" {
		return inProgressArea.Area, nil
	}
	return firstArea, nil
}

func (s *Service) buildStartIssuePlan(state *startInterviewState, activeArea string) (*startIssuePlan, error) {
	turnID, err := newTurnID()
	if err != nil {
		return nil, fmt.Errorf("new turn id: %w", err)
	}

	if state.resuming {
		resumeQuestion := ResumeQuestion(activeArea)
		return &startIssuePlan{
			resuming: true,
			issue: questionIssue{
				question: ReadinessQuestion(
					activeArea,
					resumeQuestion.TextEs,
					resumeQuestion.TextEn,
					state.flowState.QuestionNumber,
					turnID,
				),
				area: activeArea,
			},
		}, nil
	}

	return &startIssuePlan{
		resuming: false,
		issue: questionIssue{
			question: OpeningDisclaimerQuestion(
				activeArea,
				s.settings.OpeningDisclaimer.Es,
				s.settings.OpeningDisclaimer.En,
				state.flowState.QuestionNumber,
				turnID,
			),
			area: activeArea,
		},
	}, nil
}

func (s *Service) persistStartIssuePlan(
	ctx context.Context,
	sessionCode string,
	plan *startIssuePlan,
) (*IssuedQuestion, error) {
	issuedQuestion := s.issueQuestion(plan.issue)
	persistCtx, persistCancel := context.WithTimeout(ctx, s.dbTimeout)
	defer persistCancel()

	if plan.resuming {
		if _, err := s.stateStore.PrepareReadinessStep(persistCtx, sessionCode, issuedQuestion); err != nil {
			return nil, fmt.Errorf("prepare readiness step: %w", err)
		}
		return issuedQuestion, nil
	}

	if _, err := s.stateStore.PrepareDisclaimerStep(persistCtx, sessionCode, issuedQuestion); err != nil {
		return nil, fmt.Errorf("prepare disclaimer step: %w", err)
	}
	return issuedQuestion, nil
}

func (s *Service) startResultFromActiveQuestion(state *startInterviewState) *StartResult {
	overallRemaining := s.calcEffectiveTimeRemaining(state.session, state.flowState, s.nowFn())
	if overallRemaining < 0 {
		overallRemaining = 0
	}

	data := s.issuedQuestionResultData(state.flowState.ActiveQuestion, questionIssue{})
	return &StartResult{
		Question:                     data.question,
		TimerRemainingS:              overallRemaining,
		AnswerSubmitWindowRemainingS: data.answerSubmitWindowRemainingS,
		Area:                         data.area,
		Resuming:                     state.resuming,
		Language:                     state.effectiveLanguage,
	}
}

func (s *Service) startResultFromIssuedQuestion(
	state *startInterviewState,
	issue questionIssue,
	issuedQuestion *IssuedQuestion,
) *StartResult {
	overallRemaining := s.calcEffectiveTimeRemaining(state.session, nil, s.nowFn())
	if overallRemaining < 0 {
		overallRemaining = 0
	}

	data := s.issuedQuestionResultData(issuedQuestion, issue)
	return &StartResult{
		Question:                     data.question,
		TimerRemainingS:              overallRemaining,
		AnswerSubmitWindowRemainingS: data.answerSubmitWindowRemainingS,
		Area:                         data.area,
		Resuming:                     state.resuming,
		Language:                     state.effectiveLanguage,
	}
}
