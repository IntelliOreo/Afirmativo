package interview

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// UpsertAnswerJob creates or returns an existing async answer job by idempotency key.
func (s *PostgresStore) UpsertAnswerJob(ctx context.Context, params UpsertAnswerJobParams) (*AnswerJob, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO interview_answer_jobs (
		     session_code,
		     client_request_id,
		     last_request_id,
		     turn_id,
		     question_text,
		     answer_text,
		     status
		 )
		 VALUES ($1, $2, $3, $4, $5, $6, 'queued')
		 ON CONFLICT (session_code, client_request_id)
		 DO UPDATE SET
		     last_request_id = COALESCE(EXCLUDED.last_request_id, interview_answer_jobs.last_request_id),
		     updated_at = now()
		 RETURNING id, session_code, client_request_id, last_request_id, turn_id, question_text, answer_text, status,
		           result_payload, error_code, error_message, attempts, started_at, completed_at, created_at, updated_at`,
		params.SessionCode,
		params.ClientRequestID,
		nullIfEmpty(params.LastRequestID),
		params.TurnID,
		nullIfEmpty(params.QuestionText),
		params.AnswerText,
	)

	job, err := scanAnswerJob(row)
	if err != nil {
		return nil, fmt.Errorf("upsert answer job: %w", err)
	}
	return job, nil
}

// ClaimQueuedAnswerJob moves a queued job to running atomically.
// Returns nil,nil when the job is already claimed or in terminal state.
func (s *PostgresStore) ClaimQueuedAnswerJob(ctx context.Context, jobID string) (*AnswerJob, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE interview_answer_jobs
		 SET status = 'running',
		     attempts = attempts + 1,
		     started_at = COALESCE(started_at, now()),
		     updated_at = now()
		 WHERE id = $1::uuid
		   AND status = 'queued'
		 RETURNING id, session_code, client_request_id, last_request_id, turn_id, question_text, answer_text, status,
		           result_payload, error_code, error_message, attempts, started_at, completed_at, created_at, updated_at`,
		jobID,
	)

	job, err := scanAnswerJob(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("claim answer job: %w", err)
	}
	return job, nil
}

// ClaimNextQueuedAnswerJob moves the oldest queued job to running atomically.
// Returns nil,nil when no queued job exists.
func (s *PostgresStore) ClaimNextQueuedAnswerJob(ctx context.Context) (*AnswerJob, error) {
	row := s.pool.QueryRow(ctx,
		`WITH next_job AS (
		     SELECT id
		       FROM interview_answer_jobs
		      WHERE status = 'queued'
		      ORDER BY created_at ASC
		      LIMIT 1
		      FOR UPDATE SKIP LOCKED
		 )
		 UPDATE interview_answer_jobs
		    SET status = 'running',
		        attempts = attempts + 1,
		        started_at = COALESCE(started_at, now()),
		        updated_at = now()
		   FROM next_job
		  WHERE interview_answer_jobs.id = next_job.id
		 RETURNING interview_answer_jobs.id, session_code, client_request_id, last_request_id, turn_id, question_text, answer_text, status,
		           result_payload, error_code, error_message, attempts, started_at, completed_at, created_at, updated_at`,
	)

	job, err := scanAnswerJob(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("claim next answer job: %w", err)
	}
	return job, nil
}

// RequeueStaleRunningAnswerJobs marks stale running jobs as queued for retry.
func (s *PostgresStore) RequeueStaleRunningAnswerJobs(ctx context.Context, staleBefore time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE interview_answer_jobs
		    SET status = 'queued',
		        started_at = NULL,
		        completed_at = NULL,
		        updated_at = now()
		  WHERE status = 'running'
		    AND started_at IS NOT NULL
		    AND started_at < $1`,
		staleBefore.UTC(),
	)
	if err != nil {
		return 0, fmt.Errorf("requeue stale running answer jobs: %w", err)
	}
	return tag.RowsAffected(), nil
}

// GetAnswerJob returns a polling job by session and job ID.
func (s *PostgresStore) GetAnswerJob(ctx context.Context, sessionCode, jobID string) (*AnswerJob, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, session_code, client_request_id, last_request_id, turn_id, question_text, answer_text, status,
		        result_payload, error_code, error_message, attempts, started_at, completed_at, created_at, updated_at
		   FROM interview_answer_jobs
		  WHERE session_code = $1
		    AND id = $2::uuid`,
		sessionCode,
		jobID,
	)

	job, err := scanAnswerJob(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAsyncJobNotFound
		}
		return nil, fmt.Errorf("get answer job: %w", err)
	}
	return job, nil
}

// MarkAnswerJobSucceeded stores terminal success state and result payload.
func (s *PostgresStore) MarkAnswerJobSucceeded(ctx context.Context, jobID string, resultPayload []byte) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE interview_answer_jobs
		 SET status = 'succeeded',
		     result_payload = $2,
		     error_code = NULL,
		     error_message = NULL,
		     completed_at = now(),
		     updated_at = now()
		 WHERE id = $1::uuid`,
		jobID,
		resultPayload,
	)
	if err != nil {
		return fmt.Errorf("mark answer job succeeded: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAsyncJobNotFound
	}
	return nil
}

// MarkAnswerJobFailed stores terminal failure/conflict state.
func (s *PostgresStore) MarkAnswerJobFailed(ctx context.Context, params MarkAnswerJobFailedParams) error {
	status := params.Status
	if status != AsyncAnswerJobFailed && status != AsyncAnswerJobConflict && status != AsyncAnswerJobCanceled {
		status = AsyncAnswerJobFailed
	}

	tag, err := s.pool.Exec(ctx,
		`UPDATE interview_answer_jobs
		 SET status = $2,
		     error_code = $3,
		     error_message = $4,
		     completed_at = now(),
		     updated_at = now()
		 WHERE id = $1::uuid`,
		params.JobID,
		string(status),
		nullIfEmpty(params.ErrorCode),
		nullIfEmpty(params.ErrorMessage),
	)
	if err != nil {
		return fmt.Errorf("mark answer job failed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAsyncJobNotFound
	}
	return nil
}

// AppendAnswerJobFailedReason appends one retry reason string to failed_reasons_truncated.
func (s *PostgresStore) AppendAnswerJobFailedReason(ctx context.Context, jobID, reason string) error {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return nil
	}

	tag, err := s.pool.Exec(ctx,
		`UPDATE interview_answer_jobs
		 SET failed_reasons_truncated = CASE
		     WHEN COALESCE(failed_reasons_truncated, '') = '' THEN $2
		     ELSE left(failed_reasons_truncated || E'\n' || $2, 4000)
		 END,
		     updated_at = now()
		 WHERE id = $1::uuid`,
		jobID,
		trimmed,
	)
	if err != nil {
		return fmt.Errorf("append answer job failed reason: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAsyncJobNotFound
	}
	return nil
}

// IncrementAnswerJobAttempts increments attempts counter without changing status.
func (s *PostgresStore) IncrementAnswerJobAttempts(ctx context.Context, jobID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE interview_answer_jobs
		 SET attempts = attempts + 1,
		     updated_at = now()
		 WHERE id = $1::uuid`,
		jobID,
	)
	if err != nil {
		return fmt.Errorf("increment answer job attempts: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAsyncJobNotFound
	}
	return nil
}

func scanAnswerJob(row pgx.Row) (*AnswerJob, error) {
	var id pgtype.UUID
	var sessionCode string
	var clientRequestID string
	var lastRequestID pgtype.Text
	var turnID string
	var questionText pgtype.Text
	var answerText string
	var status string
	var resultPayload []byte
	var errorCode pgtype.Text
	var errorMessage pgtype.Text
	var attempts int32
	var startedAt pgtype.Timestamptz
	var completedAt pgtype.Timestamptz
	var createdAt pgtype.Timestamptz
	var updatedAt pgtype.Timestamptz

	if err := row.Scan(
		&id,
		&sessionCode,
		&clientRequestID,
		&lastRequestID,
		&turnID,
		&questionText,
		&answerText,
		&status,
		&resultPayload,
		&errorCode,
		&errorMessage,
		&attempts,
		&startedAt,
		&completedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, err
	}

	job := &AnswerJob{
		ID:              uuidToString(id),
		SessionCode:     sessionCode,
		ClientRequestID: clientRequestID,
		LastRequestID:   lastRequestID.String,
		TurnID:          turnID,
		AnswerText:      answerText,
		Status:          AsyncAnswerJobStatus(status),
		ResultPayload:   resultPayload,
		Attempts:        int(attempts),
		CreatedAt:       createdAt.Time,
		UpdatedAt:       updatedAt.Time,
	}
	if questionText.Valid {
		job.QuestionText = questionText.String
	}
	if errorCode.Valid {
		job.ErrorCode = errorCode.String
	}
	if errorMessage.Valid {
		job.ErrorMessage = errorMessage.String
	}
	if startedAt.Valid {
		t := startedAt.Time
		job.StartedAt = &t
	}
	if completedAt.Valid {
		t := completedAt.Time
		job.CompletedAt = &t
	}

	return job, nil
}
