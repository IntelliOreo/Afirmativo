-- name: CreateReport :one
INSERT INTO reports (session_code, status, content_en, content_es, strengths, strengths_es, weaknesses, weaknesses_es, recommendation, recommendation_es, question_count, duration_minutes)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: GetReportBySession :one
SELECT * FROM reports WHERE session_code = $1;

-- name: UpdateReport :exec
UPDATE reports
SET status = $2,
    content_en = $3,
    content_es = $4,
    strengths = $5,
    strengths_es = $6,
    weaknesses = $7,
    weaknesses_es = $8,
    recommendation = $9,
    recommendation_es = $10,
    question_count = $11,
    duration_minutes = $12,
    updated_at = now()
WHERE session_code = $1;
