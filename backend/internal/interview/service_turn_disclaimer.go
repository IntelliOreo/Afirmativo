package interview

import (
	"context"
	"fmt"
)

func (s *Service) handleDisclaimerTurn(ctx context.Context, sessionCode string, snapshot *turnSnapshot) (*AnswerResult, error) {
	if result, done := s.finishIfNoCurrentAreaResult(ctx, sessionCode, snapshot.currentArea, false); done {
		return result, nil
	}

	nextTurnID, err := newTurnID()
	if err != nil {
		return nil, fmt.Errorf("new turn id: %w", err)
	}

	readinessTextEs, readinessTextEn := s.disclaimerReadinessText(snapshot)
	return s.advanceNonCriterionTurn(ctx, sessionCode, snapshot, &nonCriterionAdvancePlan{
		opName:      "advance disclaimer step",
		currentStep: FlowStepDisclaimer,
		nextStep:    FlowStepReadiness,
		eventType:   "disclaimer_ack",
		question: ReadinessQuestion(
			snapshot.currentArea.Area,
			readinessTextEs,
			readinessTextEn,
			snapshot.flowState.QuestionNumber,
			nextTurnID,
		),
	})
}

func (s *Service) disclaimerReadinessText(snapshot *turnSnapshot) (string, string) {
	if snapshot.flowState.QuestionNumber > 1 {
		resumeQuestion := ResumeQuestion(snapshot.currentArea.Area)
		return resumeQuestion.TextEs, resumeQuestion.TextEn
	}
	return s.settings.ReadinessQuestion.Es, s.settings.ReadinessQuestion.En
}
