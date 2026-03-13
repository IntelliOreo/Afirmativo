package interview

import (
	"context"
	"fmt"
	"strings"

	"github.com/afirmativo/backend/internal/session"
)

// StartInterview transitions the session to interviewing,
// creates all question area rows, and returns the opening question.
func (s *Service) StartInterview(ctx context.Context, sessionCode, preferredLanguage string) (*StartResult, error) {
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
	effectiveLanguage := normalizePreferredLanguage(sess.PreferredLanguage)

	for {
		flowCtx, flowCancel := context.WithTimeout(ctx, s.dbTimeout)
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
			overallRemaining := s.calcEffectiveTimeRemaining(sess, currentFlow, s.nowFn())
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

		for _, area := range s.settings.AreaConfigs {
			createCtx, createCancel := context.WithTimeout(ctx, s.dbTimeout)
			_, err := s.stateStore.CreateQuestionArea(createCtx, sessionCode, area.Slug)
			createCancel()
			if err != nil {
				return nil, fmt.Errorf("create question area %s: %w", area.Slug, err)
			}
		}

		firstArea := s.settings.AreaConfigs[0].Slug
		setAreaCtx, setAreaCancel := context.WithTimeout(ctx, s.dbTimeout)
		if err := s.stateStore.SetAreaInProgress(setAreaCtx, sessionCode, firstArea); err != nil {
			setAreaCancel()
			return nil, fmt.Errorf("set first area in_progress: %w", err)
		}
		setAreaCancel()

		activeArea := firstArea
		activeAreaCtx, activeAreaCancel := context.WithTimeout(ctx, s.dbTimeout)
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
			persistCtx, persistCancel := context.WithTimeout(ctx, s.dbTimeout)
			_, err = s.stateStore.PrepareReadinessStep(persistCtx, sessionCode, NewIssuedQuestion(q, s.nowFn(), s.settings.AnswerTimeLimitSeconds))
			persistCancel()
			if err != nil {
				return nil, fmt.Errorf("prepare readiness step: %w", err)
			}
		} else {
			q = OpeningDisclaimerQuestion(
				activeArea,
				s.settings.OpeningDisclaimer.Es,
				s.settings.OpeningDisclaimer.En,
				currentFlow.QuestionNumber,
				turnID,
			)
			persistCtx, persistCancel := context.WithTimeout(ctx, s.dbTimeout)
			_, err = s.stateStore.PrepareDisclaimerStep(persistCtx, sessionCode, NewIssuedQuestion(q, s.nowFn(), s.settings.AnswerTimeLimitSeconds))
			persistCancel()
			if err != nil {
				return nil, fmt.Errorf("prepare disclaimer step: %w", err)
			}
		}

		overallRemaining := s.calcEffectiveTimeRemaining(sess, nil, s.nowFn())
		if overallRemaining < 0 {
			overallRemaining = 0
		}
		return &StartResult{
			Question:                     q,
			TimerRemainingS:              overallRemaining,
			AnswerSubmitWindowRemainingS: s.settings.AnswerTimeLimitSeconds,
			Area:                         activeArea,
			Resuming:                     resuming,
			Language:                     effectiveLanguage,
		}, nil
	}
}
