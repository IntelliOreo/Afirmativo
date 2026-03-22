package interview

import (
	"context"
	"fmt"
)

func (s *Service) handleDisclaimerTurn(ctx context.Context, sessionCode string, snapshot *turnSnapshot) (*AnswerResult, error) {
	if result, done, err := s.finishIfNoCurrentAreaResult(ctx, sessionCode, snapshot.currentArea, false); done {
		return result, err
	}

	plan, err := s.buildDisclaimerAdvancePlan(snapshot)
	if err != nil {
		return nil, err
	}

	return s.advanceNonCriterionTurn(ctx, sessionCode, snapshot, plan)
}

func (s *Service) buildDisclaimerAdvancePlan(snapshot *turnSnapshot) (*nonCriterionAdvancePlan, error) {
	nextTurnID, err := newTurnID()
	if err != nil {
		return nil, fmt.Errorf("new turn id: %w", err)
	}

	readinessTextEs, readinessTextEn := s.disclaimerReadinessText(snapshot)
	return &nonCriterionAdvancePlan{
		opName:      "advance disclaimer step",
		currentStep: FlowStepDisclaimer,
		nextStep:    FlowStepReadiness,
		eventType:   "disclaimer_ack",
		issue: questionIssue{
			question: ReadinessQuestion(
				snapshot.currentArea.Area,
				readinessTextEs,
				readinessTextEn,
				snapshot.flowState.QuestionNumber,
				nextTurnID,
			),
			area: snapshot.currentArea.Area,
		},
	}, nil
}

func (s *Service) disclaimerReadinessText(snapshot *turnSnapshot) (string, string) {
	if snapshot.flowState.QuestionNumber > 1 {
		resumeQuestion := ResumeQuestion(snapshot.currentArea.Area)
		return resumeQuestion.TextEs, resumeQuestion.TextEn
	}
	return s.settings.ReadinessQuestion.Es, s.settings.ReadinessQuestion.En
}
