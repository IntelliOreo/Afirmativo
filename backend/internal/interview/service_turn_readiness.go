package interview

import (
	"context"
)

func (s *Service) handleReadinessTurn(ctx context.Context, sessionCode string, snapshot *turnSnapshot) (*AnswerResult, error) {
	if result, done, err := s.finishIfNoCurrentAreaResult(ctx, sessionCode, snapshot.currentArea, false); done {
		return result, err
	}

	inputs, err := s.loadReadinessOpeningInputs(ctx, sessionCode, snapshot)
	if err != nil {
		return nil, err
	}

	selection, err := s.selectReadinessOpeningQuestion(ctx, sessionCode, snapshot, inputs)
	if err != nil {
		return nil, err
	}

	plan, err := s.buildReadinessAdvancePlan(snapshot, selection)
	if err != nil {
		return nil, err
	}

	return s.advanceNonCriterionTurn(ctx, sessionCode, snapshot, plan)
}
