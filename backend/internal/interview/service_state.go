package interview

import (
	"context"
	"fmt"
)

func (s *Service) refreshAreaState(ctx context.Context, sessionCode string) ([]QuestionArea, *QuestionArea, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, s.dbTimeout)
	defer dbCancel()

	areas, err := s.stateStore.GetAreasBySession(dbCtx, sessionCode)
	if err != nil {
		return nil, nil, fmt.Errorf("get areas by session: %w", err)
	}

	currentArea, err := s.stateStore.GetInProgressArea(dbCtx, sessionCode)
	if err != nil {
		return nil, nil, fmt.Errorf("get in-progress area: %w", err)
	}

	return areas, currentArea, nil
}
