// Store defines persistence operations for the reports table.
package report

import "context"

// Store defines persistence for reports.
type Store interface {
	// GetReportBySession returns the report for a session, or nil if not found.
	GetReportBySession(ctx context.Context, sessionCode string) (*Report, error)

	// CreateReport inserts a new report row (initially in "generating" status).
	CreateReport(ctx context.Context, r *Report) error

	// UpdateReport updates a report with the generated content.
	UpdateReport(ctx context.Context, r *Report) error
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
