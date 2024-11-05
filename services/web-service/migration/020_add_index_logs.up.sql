CREATE INDEX idx_description_tsvector ON activity_logs USING GIN (to_tsvector('english', description));
