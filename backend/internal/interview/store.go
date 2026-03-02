// Store defines persistence operations for interview-specific tables.
package interview

import "context"

// Store defines persistence for question_areas and answers.
type Store interface {
	// CreateQuestionArea inserts a new area row. Idempotent via ON CONFLICT.
	// Returns the area row, or nil if it already existed (conflict).
	CreateQuestionArea(ctx context.Context, sessionCode, area string) (*QuestionArea, error)

	// SetAreaInProgress updates a pending or pre_addressed area to in_progress.
	SetAreaInProgress(ctx context.Context, sessionCode, area string) error

	// GetInProgressArea returns the current in-progress area for a session, or nil if none.
	GetInProgressArea(ctx context.Context, sessionCode string) (*QuestionArea, error)

	// GetAreasBySession returns all question_area rows for a session.
	GetAreasBySession(ctx context.Context, sessionCode string) ([]QuestionArea, error)

	// IncrementAreaQuestions increments questions_count by 1 for the given area.
	IncrementAreaQuestions(ctx context.Context, sessionCode, area string) error

	// CompleteArea marks an area as complete with area_ended_at = now().
	CompleteArea(ctx context.Context, sessionCode, area string) error

	// MarkAreaInsufficient marks an area as insufficient with area_ended_at = now().
	MarkAreaInsufficient(ctx context.Context, sessionCode, area string) error

	// MarkAreaPreAddressed marks a pending area as pre_addressed with evidence.
	MarkAreaPreAddressed(ctx context.Context, sessionCode, area, evidence string) error

	// MarkAreaNotAssessed marks a pending/pre_addressed area as not_assessed.
	MarkAreaNotAssessed(ctx context.Context, sessionCode, area string) error

	// SaveAnswer inserts a new answer row and returns it.
	SaveAnswer(ctx context.Context, params SaveAnswerParams) (*Answer, error)

	// GetAnswersBySession returns all answers for a session ordered by created_at.
	GetAnswersBySession(ctx context.Context, sessionCode string) ([]Answer, error)

	// GetAnswerCount returns the number of answers for a session.
	GetAnswerCount(ctx context.Context, sessionCode string) (int, error)
}

// SaveAnswerParams holds the inputs for saving an answer.
type SaveAnswerParams struct {
	SessionCode  string
	Area         string
	QuestionText string
	TranscriptEs string
	TranscriptEn string
	AiEvaluation []byte // JSON blob
	Sufficiency  string
}
