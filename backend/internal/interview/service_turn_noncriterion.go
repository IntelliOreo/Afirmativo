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
	question    *Question
	substituted bool
}

func (s *Service) advanceNonCriterionTurn(
	ctx context.Context,
	sessionCode string,
	snapshot *turnSnapshot,
	plan *nonCriterionAdvancePlan,
) (*AnswerResult, error) {
	issuedQuestion := NewIssuedQuestion(plan.question, s.nowFn(), s.settings.AnswerTimeLimitSeconds)

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
		resolvedIssuedQuestion(nextFlow, issuedQuestion),
		plan.question,
		snapshot.timeRemainingS,
		plan.substituted,
	), nil
}
