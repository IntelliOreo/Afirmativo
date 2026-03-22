package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore implements Store backed by PostgreSQL.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a new PostgresStore.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// CleanUpOlderThan deletes all session-scoped data for sessions older than cutoff.
func (s *PostgresStore) CleanUpOlderThan(ctx context.Context, cutoff time.Time) (DeletedRows, error) {
	const cleanupQuery = `
WITH old_sessions AS (
	SELECT session_code
	FROM sessions
	WHERE created_at < $1
),
deleted_answers AS (
	DELETE FROM answers a
	USING old_sessions os
	WHERE a.session_code = os.session_code
	RETURNING 1
),
deleted_interview_events AS (
	DELETE FROM interview_events ie
	USING old_sessions os
	WHERE ie.session_code = os.session_code
	RETURNING 1
),
deleted_question_areas AS (
	DELETE FROM question_areas qa
	USING old_sessions os
	WHERE qa.session_code = os.session_code
	RETURNING 1
),
deleted_reports AS (
	DELETE FROM reports r
	USING old_sessions os
	WHERE r.session_code = os.session_code
	RETURNING 1
),
deleted_sessions AS (
	DELETE FROM sessions s
	USING old_sessions os
	WHERE s.session_code = os.session_code
	RETURNING 1
)
SELECT
	(SELECT COUNT(*) FROM deleted_answers),
	(SELECT COUNT(*) FROM deleted_interview_events),
	(SELECT COUNT(*) FROM deleted_question_areas),
	(SELECT COUNT(*) FROM deleted_reports),
	(SELECT COUNT(*) FROM deleted_sessions);
`

	var deleted DeletedRows
	if err := s.pool.QueryRow(ctx, cleanupQuery, cutoff.UTC()).Scan(
		&deleted.Answers,
		&deleted.InterviewEvents,
		&deleted.QuestionAreas,
		&deleted.Reports,
		&deleted.Sessions,
	); err != nil {
		return DeletedRows{}, fmt.Errorf("clean up old sessions: %w", err)
	}

	return deleted, nil
}
