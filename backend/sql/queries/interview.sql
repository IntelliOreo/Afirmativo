-- Queries for answers table.
-- Table not yet migrated — queries will be uncommented after migration.

-- name: SaveAnswer :one
-- INSERT INTO answers (session_code, question_id, transcript_es, transcript_en, ai_evaluation, sufficiency)
-- VALUES ($1, $2, $3, $4, $5, $6)
-- RETURNING *;

-- name: GetAnswersBySession :many
-- SELECT * FROM answers WHERE session_code = $1 ORDER BY created_at;

-- name: GetAnswerCount :one
-- SELECT count(*) FROM answers WHERE session_code = $1;
