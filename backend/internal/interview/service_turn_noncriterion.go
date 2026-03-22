package interview

import (
	"context"
	"fmt"
)

type nonCriterionAdvancePlan struct {
	opName      string
	currentStep FlowStep
	nextStep    FlowStep
	eventType   string
	issue       questionIssue
}

func (s *Service) advanceNonCriterionTurn(
	ctx context.Context,
	sessionCode string,
	snapshot *turnSnapshot,
	plan *nonCriterionAdvancePlan,
) (*AnswerResult, error) {
	issuedQuestion := s.issueQuestion(plan.issue)

	advanceCtx, advanceCancel := context.WithTimeout(ctx, s.dbTimeout)
	nextFlow, err := s.stateStore.AdvanceNonCriterionStep(advanceCtx, AdvanceNonCriterionStepParams{
		SessionCode:        sessionCode,
		ExpectedTurnID:     snapshot.turnID,
		CurrentStep:        plan.currentStep,
		NextStep:           plan.nextStep,
		EventType:          plan.eventType,
		AnswerText:         snapshot.answerText,
		NextIssuedQuestion: issuedQuestion,
	})
	advanceCancel()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", plan.opName, err)
	}

	return s.buildTurnAnswerResult(
		s.resolvedIssuedQuestionResultData(nextFlow, issuedQuestion, plan.issue),
		snapshot.timeRemainingS,
	), nil
}
