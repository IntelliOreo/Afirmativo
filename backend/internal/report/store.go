// Store defines persistence operations for the reports table.
package report

import (
	"context"
	"time"
)

// Store defines persistence for reports.
type Store interface {
	// GetReportBySession returns the report for a session, or nil if not found.
	GetReportBySession(ctx context.Context, sessionCode string) (*Report, error)

	// CreateReport inserts a new report row.
	CreateReport(ctx context.Context, r *Report) error

	// SetReportQueued resets one report to the queued state.
	SetReportQueued(ctx context.Context, sessionCode string, resetAttempts bool, lastRequestID string) error

	// SetReportLastRequestID updates request correlation without changing report status.
	SetReportLastRequestID(ctx context.Context, sessionCode, lastRequestID string) error

	// ClaimQueuedReport moves a queued report to running atomically.
	ClaimQueuedReport(ctx context.Context, sessionCode string) (*Report, error)

	// ClaimNextQueuedReport atomically claims the next queued report. Returns nil,nil when no queued report exists.
	ClaimNextQueuedReport(ctx context.Context) (*Report, error)

	// RequeueStaleRunningReports moves stale running reports back to queued.
	RequeueStaleRunningReports(ctx context.Context, staleBefore time.Time) (int64, error)

	// MarkReportReady stores a completed report payload.
	MarkReportReady(ctx context.Context, r *Report) error

	// MarkReportFailed stores a terminal failed state.
	MarkReportFailed(ctx context.Context, sessionCode, errorCode, errorMessage string) error
}

// InterviewDataProvider provides read access to interview answers and question areas.
// Implemented by the interview package's postgres store, injected from main.go.
type InterviewDataProvider interface {
	// GetAreasBySession returns all question_area rows for a session.
	GetAreasBySession(ctx context.Context, sessionCode string) ([]InterviewAreaSnapshot, error)

	// GetAnswersBySession returns all answers for a session ordered by created_at.
	GetAnswersBySession(ctx context.Context, sessionCode string) ([]InterviewAnswerSnapshot, error)

	// GetAnswerCount returns the number of answers for a session.
	GetAnswerCount(ctx context.Context, sessionCode string) (int, error)
}

// SessionProvider provides read access to session data.
type SessionProvider interface {
	GetSessionByCode(ctx context.Context, sessionCode string) (*SessionInfo, error)
}

// InterviewAreaSnapshot is a simplified view of a question area for report generation.
type InterviewAreaSnapshot struct {
	Area                 string
	Status               string
	PreAddressedEvidence string
}

// InterviewAnswerSnapshot is a simplified view of an answer for report generation.
type InterviewAnswerSnapshot struct {
	Area         string
	QuestionText string
	TranscriptEs string
	TranscriptEn string
	AIEvaluation *AnswerEvaluation
	Sufficiency  string
}

type AnswerEvaluation struct {
	EvidenceSummary string
	Recommendation  string
}

// SessionInfo is the minimal session data needed for report generation.
type SessionInfo struct {
	SessionCode        string
	Status             string
	PreferredLanguage  string
	InterviewStartedAt int64 // unix timestamp
	EndedAt            int64 // unix timestamp
}
