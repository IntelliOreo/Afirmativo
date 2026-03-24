ALTER TABLE interview_answer_jobs
    ADD COLUMN last_request_id TEXT;

ALTER TABLE reports
    ADD COLUMN last_request_id TEXT;
