-- name: CreateReport :one
INSERT INTO reports (session_code, status, content_en, content_es, strengths, weaknesses, recommendation, question_count, duration_minutes)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetReportBySession :one
SELECT * FROM reports WHERE session_code = $1;

-- name: UpdateReport :exec
UPDATE reports
SET status = $2,
    content_en = $3,
    content_es = $4,
    strengths = $5,
    weaknesses = $6,
    recommendation = $7,
    question_count = $8,
    duration_minutes = $9,
    updated_at = now()
WHERE session_code = $1;
