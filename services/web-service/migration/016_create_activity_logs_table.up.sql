CREATE TABLE activity_logs (
    id BIGSERIAL PRIMARY KEY,
    user_id INT REFERENCES users(id) ON DELETE SET NULL,
    activity_type VARCHAR(50) NOT NULL, -- 'login', 'logout', 'update_profile', etc.
    status VARCHAR(20) DEFAULT 'success', -- 'success' or 'failure'
    description TEXT, -- Additional details, e.g., "Failed login due to incorrect password"
    ip_address INET, -- Logs the user's IP address
    user_agent TEXT, -- User's device or browser information
    extra JSONB, -- Additional data (like id of the updated user,..) in JSON format
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- Optional: Indexing for efficient querying
CREATE INDEX idx_activity_logs_user_id ON activity_logs(user_id);
CREATE INDEX idx_activity_logs_activity_type ON activity_logs(activity_type);
CREATE INDEX idx_activity_logs_status ON activity_logs(status);
CREATE INDEX idx_activity_logs_created_at ON activity_logs(created_at);