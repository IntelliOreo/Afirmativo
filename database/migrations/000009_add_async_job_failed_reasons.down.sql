-- Migration 000009 rollback.
ALTER TABLE interview_answer_jobs
    DROP COLUMN IF EXISTS failed_reasons_truncated;
