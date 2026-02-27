// Service layer for interview operations.
// StartInterview: sets session to interviewing, returns first question.
package interview

import (
	"context"

	"github.com/afirmativo/backend/internal/session"
)

// SessionStarter transitions a session from 'created' to 'interviewing'.
type SessionStarter interface {
	StartSession(ctx context.Context, sessionCode string) (*session.Session, error)
}

// Service contains interview business logic.
type Service struct {
	sessionStarter SessionStarter
}

// NewService creates a Service with the given session starter.
func NewService(ss SessionStarter) *Service {
	return &Service{sessionStarter: ss}
}

// StartResult holds the output of a successful interview start.
type StartResult struct {
	Question        *Question
	TimerRemainingS int
}

// StartInterview transitions the session to interviewing and returns the first question.
func (s *Service) StartInterview(ctx context.Context, sessionCode string) (*StartResult, error) {
	sess, err := s.sessionStarter.StartSession(ctx, sessionCode)
	if err != nil {
		return nil, err
	}
	return &StartResult{
		Question:        FirstQuestion(),
		TimerRemainingS: sess.TimerSeconds,
	}, nil
}
