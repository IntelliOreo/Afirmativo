-- Migration 000004: Add pre_addressed_evidence to question_areas, change default status to pending.

-- Add column for storing AI reasoning when a criterion is flagged as covered elsewhere.
ALTER TABLE question_areas ADD COLUMN pre_addressed_evidence TEXT;

-- Change default status from 'in_progress' to 'pending'.
-- New rows start as pending; the backend explicitly sets in_progress on the current criterion.
ALTER TABLE question_areas ALTER COLUMN status SET DEFAULT 'pending';
