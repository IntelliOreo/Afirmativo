ALTER TABLE reports
    ADD COLUMN error_code TEXT,
    ADD COLUMN error_message TEXT,
    ADD COLUMN attempts INT NOT NULL DEFAULT 0,
    ADD COLUMN started_at TIMESTAMPTZ,
    ADD COLUMN completed_at TIMESTAMPTZ;

UPDATE reports
   SET status = 'queued'
 WHERE status = 'generating';

ALTER TABLE reports
    ALTER COLUMN status SET DEFAULT 'queued';

ALTER TABLE reports
    ADD CONSTRAINT chk_reports_status
    CHECK (status IN ('queued', 'running', 'ready', 'failed'));

CREATE INDEX idx_reports_status_updated_at ON reports(status, updated_at);
CREATE INDEX idx_reports_running_started_at ON reports(started_at) WHERE status = 'running';
