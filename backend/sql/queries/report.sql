-- Queries for reports table.
-- Table not yet migrated — queries will be uncommented after migration.

-- name: CreateReport :one
-- INSERT INTO reports (session_code, status, content_en, content_es, strengths, weaknesses, recommendation)
-- VALUES ($1, $2, $3, $4, $5, $6, $7)
-- RETURNING *;

-- name: GetReportBySession :one
-- SELECT * FROM reports WHERE session_code = $1;
