package admin

import (
	"context"
	"time"
)

// DeletedRows contains per-table row deletion counts from a cleanup run.
type DeletedRows struct {
	Answers         int64 `json:"answers"`
	InterviewEvents int64 `json:"interview_events"`
	QuestionAreas   int64 `json:"question_areas"`
	Reports         int64 `json:"reports"`
	Sessions        int64 `json:"sessions"`
}

// Total returns the total deleted rows across all cleaned tables.
func (d DeletedRows) Total() int64 {
	return d.Answers + d.InterviewEvents + d.QuestionAreas + d.Reports + d.Sessions
}

// Store defines persistence operations for admin maintenance jobs.
type Store interface {
	// CleanUpOlderThan deletes data belonging to sessions older than cutoff.
	CleanUpOlderThan(ctx context.Context, cutoff time.Time) (DeletedRows, error)
}
