-- Migration 000009: Add truncated AI retry diagnostics for async interview jobs.
ALTER TABLE interview_answer_jobs
    ADD COLUMN IF NOT EXISTS failed_reasons_truncated TEXT NOT NULL DEFAULT '';
