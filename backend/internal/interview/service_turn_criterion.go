package interview

import (
	"context"
	"fmt"
	"strings"
)

func (s *Service) handleCriterionTurn(ctx context.Context, sessionCode string, snapshot *turnSnapshot) (*AnswerResult, error) {
	if s.finishIfNoCurrentArea(ctx, sessionCode, snapshot.currentArea, true) {
		return &AnswerResult{Done: true, TimerRemainingS: 0, AnswerSubmitWindowRemainingS: 0}, nil
	}

	inputs, err := s.loadCriterionTurnAnswers(ctx, sessionCode, snapshot)
	if err != nil {
		return nil, err
	}

	evaluation, err := s.evaluateCriterionTurn(ctx, sessionCode, snapshot, inputs)
	if err != nil {
		return nil, err
	}

	plan := s.planCriterionTransition(snapshot, inputs.answers, evaluation)
	nextQuestion, err := s.buildNextCriterionQuestion(ctx, sessionCode, snapshot, plan, evaluation)
	if err != nil {
		return nil, err
	}

	if err := s.persistCriterionTurn(ctx, sessionCode, snapshot, evaluation, plan, nextQuestion); err != nil {
		return nil, err
	}
	if _, _, err := s.refreshAreaState(ctx, sessionCode); err != nil {
		return nil, fmt.Errorf("refresh areas after criterion: %w", err)
	}

	if strings.TrimSpace(plan.nextArea) == "" {
		s.finishSession(ctx, sessionCode)
		return &AnswerResult{Done: true, TimerRemainingS: 0, AnswerSubmitWindowRemainingS: 0}, nil
	}

	return &AnswerResult{
		Done: false,
		NextQuestion: func() *Question {
			if nextQuestion.issuedQuestion != nil {
				return &nextQuestion.issuedQuestion.Question
			}
			return nextQuestion.question
		}(),
		TimerRemainingS:              snapshot.timeRemainingS,
		AnswerSubmitWindowRemainingS: nextQuestion.issuedQuestion.SubmitWindowRemaining(s.nowFn()),
		Substituted:                  nextQuestion.substituted,
	}, nil
}
