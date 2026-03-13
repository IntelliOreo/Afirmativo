package interview

import (
	"context"
	"log/slog"
)

// finishSession marks the session as completed. Logs on error but does not
// propagate - the interview result has already been determined.
func (s *Service) finishSession(ctx context.Context, sessionCode string) {
	if err := s.sessionCompleter.CompleteSession(ctx, sessionCode); err != nil {
		slog.Error("failed to complete session", "session", sessionCode, "error", err)
	}
}

func (s *Service) finishOnTimeout(ctx context.Context, sessionCode string, areas []QuestionArea) (*AnswerResult, error) {
	s.markRemainingNotAssessed(ctx, sessionCode, areas)
	if err := s.stateStore.MarkFlowDone(ctx, sessionCode); err != nil {
		slog.Warn("failed to mark flow done on timeout", "session", sessionCode, "error", err)
	}
	s.finishSession(ctx, sessionCode)
	return doneAnswerResult(false), nil
}

func (s *Service) finishIfNoCurrentArea(ctx context.Context, sessionCode string, currentArea *QuestionArea, markDone bool) bool {
	if currentArea != nil {
		return false
	}
	if markDone {
		if err := s.stateStore.MarkFlowDone(ctx, sessionCode); err != nil {
			slog.Warn("failed to mark flow done with no current area", "session", sessionCode, "error", err)
		}
	}
	s.finishSession(ctx, sessionCode)
	return true
}

func (s *Service) finishIfNoCurrentAreaResult(
	ctx context.Context,
	sessionCode string,
	currentArea *QuestionArea,
	markDone bool,
) (*AnswerResult, bool) {
	if !s.finishIfNoCurrentArea(ctx, sessionCode, currentArea, markDone) {
		return nil, false
	}
	return doneAnswerResult(false), true
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
