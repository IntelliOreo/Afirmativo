package interview

import (
	"context"
	"fmt"
	"strings"
)

func (s *Service) handleCriterionTurn(ctx context.Context, sessionCode string, snapshot *turnSnapshot) (*AnswerResult, error) {
	if result, done := s.finishIfNoCurrentAreaResult(ctx, sessionCode, snapshot.currentArea, true); done {
		return result, nil
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
		return doneAnswerResult(false), nil
	}

	return s.buildTurnAnswerResult(
		s.issuedQuestionResultData(nextQuestion.issuedQuestion, questionIssue{
			question:    nextQuestion.question,
			area:        nextQuestion.question.Area,
			substituted: nextQuestion.substituted,
		}),
		snapshot.timeRemainingS,
	), nil
}
