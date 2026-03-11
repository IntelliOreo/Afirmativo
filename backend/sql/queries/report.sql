-- name: CreateReport :one
INSERT INTO reports (
    session_code, status, content_en, content_es, strengths, strengths_es, weaknesses, weaknesses_es,
    recommendation, recommendation_es, question_count, duration_minutes, error_code, error_message,
    attempts, started_at, completed_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
RETURNING *;

-- name: GetReportBySession :one
SELECT * FROM reports WHERE session_code = $1;
