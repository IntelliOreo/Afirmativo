-- Revert migration 000004.
ALTER TABLE question_areas DROP COLUMN IF EXISTS pre_addressed_evidence;
ALTER TABLE question_areas ALTER COLUMN status SET DEFAULT 'in_progress';
