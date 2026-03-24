ALTER TABLE reports
    ADD COLUMN strengths_es JSONB,
    ADD COLUMN weaknesses_es JSONB,
    ADD COLUMN recommendation_es TEXT;
