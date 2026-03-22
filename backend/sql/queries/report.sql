-- name: CreateReport :one
INSERT INTO reports (
    session_code, status, content_en, content_es, strengths, strengths_es, weaknesses, weaknesses_es,
    recommendation, recommendation_es, question_count, duration_minutes, error_code, error_message,
    attempts, started_at, completed_at, last_request_id
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
RETURNING id, session_code, status, content_en, content_es, strengths, weaknesses, recommendation, question_count, duration_minutes, created_at, updated_at, strengths_es, weaknesses_es, recommendation_es, error_code, error_message, attempts, started_at, completed_at, last_request_id;

-- name: GetReportBySession :one
SELECT id, session_code, status, content_en, content_es, strengths, weaknesses, recommendation, question_count, duration_minutes, created_at, updated_at, strengths_es, weaknesses_es, recommendation_es, error_code, error_message, attempts, started_at, completed_at, last_request_id
FROM reports
WHERE session_code = $1;
