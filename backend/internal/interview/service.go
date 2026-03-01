// Service layer for interview operations.
// StartInterview: sets session to interviewing, creates first area, returns first question.
// SubmitAnswer: saves answer, gets next question from AI (or mock), returns it.
package interview

import (
	"context"
	"fmt"
	"time"

	"github.com/afirmativo/backend/internal/session"
)

const (
	dbTimeout = 5 * time.Second
	aiTimeout = 30 * time.Second
)

// SessionStarter transitions a session to 'interviewing'.
type SessionStarter interface {
	StartSession(ctx context.Context, sessionCode string) (*session.Session, error)
}

// AIClient calls an external API (mock server now, real AI later) to get the next question.
type AIClient interface {
	NextQuestion(ctx context.Context, questionNumber int) (*Question, error)
}

// Service contains interview business logic.
type Service struct {
	sessionStarter SessionStarter
	store          Store
	aiClient       AIClient
}

// NewService creates a Service with the given dependencies.
func NewService(ss SessionStarter, store Store, ai AIClient) *Service {
	return &Service{sessionStarter: ss, store: store, aiClient: ai}
}

// StartResult holds the output of a successful interview start.
type StartResult struct {
	Question        *Question
	TimerRemainingS int
	Area            string
}

// StartInterview transitions the session to interviewing,
// creates the first question area, and returns the opening question.
func (s *Service) StartInterview(ctx context.Context, sessionCode string) (*StartResult, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	sess, err := s.sessionStarter.StartSession(dbCtx, sessionCode)
	if err != nil {
		return nil, err
	}

	remaining := sess.InterviewBudgetSeconds - sess.InterviewLapsedSeconds

	// Create the first question area (idempotent — ON CONFLICT DO NOTHING).
	firstArea := DefinedAreas[0].Slug
	_, err = s.store.CreateQuestionArea(dbCtx, sessionCode, firstArea)
	if err != nil {
		return nil, fmt.Errorf("create first question area: %w", err)
	}

	q := FirstQuestion()

	return &StartResult{
		Question:        q,
		TimerRemainingS: remaining,
		Area:            firstArea,
	}, nil
}

// AnswerResult holds the output of a submitted answer.
type AnswerResult struct {
	Done            bool
	NextQuestion    *Question
	TimerRemainingS int
}

// SubmitAnswer accepts the user's text answer, calls the question generator
// to get the next question, and returns it.
func (s *Service) SubmitAnswer(ctx context.Context, sessionCode string, answerText string, questionNumber int) (*AnswerResult, error) {
	nextNum := questionNumber + 1

	if nextNum > EstimatedTotalQuestions {
		return &AnswerResult{Done: true}, nil
	}

	aiCtx, aiCancel := context.WithTimeout(ctx, aiTimeout)
	defer aiCancel()

	q, err := s.aiClient.NextQuestion(aiCtx, nextNum)
	if err != nil {
		return nil, fmt.Errorf("generate next question: %w", err)
	}

	return &AnswerResult{
		Done:         false,
		NextQuestion: q,
	}, nil
}
