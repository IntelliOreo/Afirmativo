// Store defines persistence operations for interview-specific tables.
package interview

import "context"

// Store defines persistence for question_areas.
type Store interface {
	// CreateQuestionArea inserts a new in_progress area. Idempotent via ON CONFLICT.
	// Returns the area row, or nil if it already existed (conflict).
	CreateQuestionArea(ctx context.Context, sessionCode, area string) (*QuestionArea, error)

	// GetInProgressArea returns the current in-progress area for a session, or nil if none.
	GetInProgressArea(ctx context.Context, sessionCode string) (*QuestionArea, error)

	// GetAreasBySession returns all question_area rows for a session.
	GetAreasBySession(ctx context.Context, sessionCode string) ([]QuestionArea, error)
}
