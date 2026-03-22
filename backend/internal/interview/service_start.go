package interview

import (
	"context"
)

// StartInterview transitions the session to interviewing,
// creates all question area rows, and returns the opening question.
func (s *Service) StartInterview(ctx context.Context, sessionCode, preferredLanguage string) (*StartResult, error) {
	startState, err := s.loadStartInterviewState(ctx, sessionCode, preferredLanguage)
	if err != nil {
		return nil, err
	}

	if startState.flowState.ActiveQuestion != nil {
		return s.startResultFromActiveQuestion(startState), nil
	}

	activeArea, err := s.ensureStartAreaState(ctx, sessionCode)
	if err != nil {
		return nil, err
	}

	plan, err := s.buildStartIssuePlan(startState, activeArea)
	if err != nil {
		return nil, err
	}

	issuedQuestion, err := s.persistStartIssuePlan(ctx, sessionCode, plan)
	if err != nil {
		return nil, err
	}

	return s.startResultFromIssuedQuestion(startState, plan.issue, issuedQuestion), nil
}
