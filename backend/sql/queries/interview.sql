-- Queries for interview tables (question_areas + answers).

-- name: CreateQuestionArea :one
INSERT INTO question_areas (session_code, area)
VALUES ($1, $2)
ON CONFLICT (session_code, area) DO NOTHING
RETURNING *;

-- name: SetAreaInProgress :exec
UPDATE question_areas
SET status = 'in_progress', area_started_at = now()
WHERE session_code = $1 AND area = $2 AND status IN ('pending', 'pre_addressed');

-- name: GetInProgressArea :one
SELECT * FROM question_areas
WHERE session_code = $1 AND status = 'in_progress'
LIMIT 1;

-- name: GetAreasBySession :many
SELECT * FROM question_areas
WHERE session_code = $1
ORDER BY area_started_at;

-- name: SaveAnswer :one
INSERT INTO answers (session_code, area, question_text, audio_urls, transcript_es, transcript_en, ai_evaluation, sufficiency, flags)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetAnswersBySession :many
SELECT * FROM answers WHERE session_code = $1 ORDER BY created_at;

-- name: GetAnswerCount :one
SELECT count(*) FROM answers WHERE session_code = $1;

-- name: IncrementAreaQuestions :exec
UPDATE question_areas
SET questions_count = questions_count + 1
WHERE session_code = $1 AND area = $2;

-- name: CompleteArea :exec
UPDATE question_areas
SET status = 'complete', area_ended_at = now()
WHERE session_code = $1 AND area = $2;

-- name: MarkAreaInsufficient :exec
UPDATE question_areas
SET status = 'insufficient', area_ended_at = now()
WHERE session_code = $1 AND area = $2;

-- name: MarkAreaPreAddressed :exec
UPDATE question_areas
SET status = 'pre_addressed', pre_addressed_evidence = $3
WHERE session_code = $1 AND LOWER(area) = LOWER($2) AND status = 'pending';

-- name: MarkAreaNotAssessed :exec
UPDATE question_areas
SET status = 'not_assessed', area_ended_at = now()
WHERE session_code = $1 AND area = $2 AND status IN ('pending', 'pre_addressed');
