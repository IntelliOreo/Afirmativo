// Store defines persistence operations for interview-specific tables.
package interview

import (
	"context"
	"time"
)

// InterviewStateStore defines persistence for interview flow and answers.
type InterviewStateStore interface {
	// CreateQuestionArea inserts a new area row. Idempotent via ON CONFLICT.
	// Returns the area row, or nil if it already existed (conflict).
	CreateQuestionArea(ctx context.Context, sessionCode, area string) (*QuestionArea, error)

	// SetAreaInProgress updates a pending or pre_addressed area to in_progress.
	SetAreaInProgress(ctx context.Context, sessionCode, area string) error

	// GetInProgressArea returns the current in-progress area for a session, or nil if none.
	GetInProgressArea(ctx context.Context, sessionCode string) (*QuestionArea, error)

	// GetAreasBySession returns all question_area rows for a session.
	GetAreasBySession(ctx context.Context, sessionCode string) ([]QuestionArea, error)

	// MarkAreaNotAssessed marks a pending/pre_addressed area as not_assessed.
	MarkAreaNotAssessed(ctx context.Context, sessionCode, area string) error

	// GetAnswersBySession returns all answers for a session ordered by created_at.
	GetAnswersBySession(ctx context.Context, sessionCode string) ([]Answer, error)

	// GetAnswerCount returns the number of answers for a session.
	GetAnswerCount(ctx context.Context, sessionCode string) (int, error)

	// GetFlowState returns the session's interview flow pointer.
	GetFlowState(ctx context.Context, sessionCode string) (*FlowState, error)

	// PrepareDisclaimerStep sets flow_step=disclaimer and persists the issued question snapshot.
	PrepareDisclaimerStep(ctx context.Context, sessionCode string, issuedQuestion *IssuedQuestion) (*FlowState, error)

	// PrepareReadinessStep sets flow_step=readiness and persists the issued question snapshot.
	PrepareReadinessStep(ctx context.Context, sessionCode string, issuedQuestion *IssuedQuestion) (*FlowState, error)

	// AdvanceNonCriterionStep records a non-criterion event and advances flow state atomically.
	AdvanceNonCriterionStep(ctx context.Context, params AdvanceNonCriterionStepParams) (*FlowState, error)

	// ProcessCriterionTurn persists answer + area transitions + flow pointer atomically.
	ProcessCriterionTurn(ctx context.Context, params ProcessCriterionTurnParams) (*ProcessCriterionTurnResult, error)

	// MarkFlowDone clears expected turn and marks flow as done.
	MarkFlowDone(ctx context.Context, sessionCode string) error
}

// AsyncAnswerJobStore defines persistence for async submit/poll job workflow.
type AsyncAnswerJobStore interface {
	// UpsertAnswerJob creates an async answer job or returns the existing one for an idempotency key.
	UpsertAnswerJob(ctx context.Context, params UpsertAnswerJobParams) (*AnswerJob, error)

	// ClaimQueuedAnswerJob marks a queued job as running and returns it. Returns nil,nil if already claimed/terminal.
	ClaimQueuedAnswerJob(ctx context.Context, jobID string) (*AnswerJob, error)

	// ListQueuedAnswerJobIDs returns queued job IDs ordered oldest-first.
	ListQueuedAnswerJobIDs(ctx context.Context, limit int) ([]string, error)

	// RequeueStaleRunningAnswerJobs moves stale running jobs back to queued status.
	RequeueStaleRunningAnswerJobs(ctx context.Context, staleBefore time.Time) (int64, error)

	// GetAnswerJob returns a polling job by session+job id.
	GetAnswerJob(ctx context.Context, sessionCode, jobID string) (*AnswerJob, error)

	// MarkAnswerJobSucceeded stores terminal success state and result payload.
	MarkAnswerJobSucceeded(ctx context.Context, jobID string, resultPayload []byte) error

	// MarkAnswerJobFailed stores terminal failure or conflict state.
	MarkAnswerJobFailed(ctx context.Context, params MarkAnswerJobFailedParams) error

	// AppendAnswerJobFailedReason appends one truncated retry failure reason string.
	AppendAnswerJobFailedReason(ctx context.Context, jobID, reason string) error

	// IncrementAnswerJobAttempts increments attempts without changing status.
	IncrementAnswerJobAttempts(ctx context.Context, jobID string) error
}

// Store composes the interview persistence ports required by the service layer.
type Store interface {
	InterviewStateStore
	AsyncAnswerJobStore
}

// SaveAnswerParams holds the inputs for saving an answer.
type SaveAnswerParams struct {
	SessionCode      string
	Area             string
	QuestionText     string
	TranscriptEs     string
	TranscriptEn     string
	AIEvaluationJSON []byte // JSON blob
	Sufficiency      string
}

type AdvanceNonCriterionStepParams struct {
	SessionCode        string
	ExpectedTurnID     string
	CurrentStep        FlowStep
	NextStep           FlowStep
	EventType          string
	AnswerText         string
	NextIssuedQuestion *IssuedQuestion
}

type ProcessCriterionTurnParams struct {
	SessionCode        string
	ExpectedTurnID     string
	CurrentArea        string
	QuestionText       string
	AnswerText         string
	PreferredLanguage  string
	Evaluation         *Evaluation
	PreAddressed       []PreAddressedArea
	Decision           CriterionTurnDecision
	NextArea           string
	NextIssuedQuestion *IssuedQuestion
}

type PreAddressedArea struct {
	Slug     string
	Evidence string
}

type ProcessCriterionTurnResult struct {
	NewCount int
}

type UpsertAnswerJobParams struct {
	SessionCode     string
	ClientRequestID string
	TurnID          string
	QuestionText    string
	AnswerText      string
}

type MarkAnswerJobFailedParams struct {
	JobID        string
	Status       AsyncAnswerJobStatus
	ErrorCode    string
	ErrorMessage string
}
