package interview

import (
	"context"
	"fmt"
	"log/slog"
)

// finishSession marks the session as completed. Returns an error if
// CompleteSession fails so callers (especially async jobs) can retry.
func (s *Service) finishSession(ctx context.Context, sessionCode string) error {
	if err := s.sessionCompleter.CompleteSession(ctx, sessionCode); err != nil {
		slog.Error("failed to complete session", "session", sessionCode, "error", err)
		return fmt.Errorf("%w: %v", ErrSessionCompleteFailed, err)
	}
	return nil
}

func (s *Service) finishOnTimeout(ctx context.Context, sessionCode string, areas []QuestionArea) (*AnswerResult, error) {
	s.markRemainingNotAssessed(ctx, sessionCode, areas)
	if err := s.stateStore.MarkFlowDone(ctx, sessionCode); err != nil {
		slog.Warn("failed to mark flow done on timeout", "session", sessionCode, "error", err)
	}
	if err := s.finishSession(ctx, sessionCode); err != nil {
		return nil, err
	}
	return doneAnswerResult(false), nil
}

func (s *Service) finishIfNoCurrentArea(ctx context.Context, sessionCode string, currentArea *QuestionArea, markDone bool) (bool, error) {
	if currentArea != nil {
		return false, nil
	}
	if markDone {
		if err := s.stateStore.MarkFlowDone(ctx, sessionCode); err != nil {
			slog.Warn("failed to mark flow done with no current area", "session", sessionCode, "error", err)
		}
	}
	if err := s.finishSession(ctx, sessionCode); err != nil {
		return true, err
	}
	return true, nil
}

func (s *Service) finishIfNoCurrentAreaResult(
	ctx context.Context,
	sessionCode string,
	currentArea *QuestionArea,
	markDone bool,
) (*AnswerResult, bool, error) {
	finished, err := s.finishIfNoCurrentArea(ctx, sessionCode, currentArea, markDone)
	if !finished {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	return doneAnswerResult(false), true, nil
}

func (s *Service) markRemainingNotAssessed(ctx context.Context, sessionCode string, areas []QuestionArea) {
	dbCtx, dbCancel := context.WithTimeout(ctx, s.dbTimeout)
	defer dbCancel()

	for _, a := range areas {
		if isAreaUnresolved(a.Status) {
			if err := s.stateStore.MarkAreaNotAssessed(dbCtx, sessionCode, a.Area); err != nil {
				slog.Warn("failed to mark not_assessed", "area", a.Area, "error", err)
			}
		}
	}
}
