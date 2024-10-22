CREATE TABLE IF NOT EXISTS user_activity_history (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL,  -- Foreign key reference to users table
    activity_type VARCHAR(100) NOT NULL,  -- e.g., login, logout, add_user, delete_code, upload_code, trigger_draw
    activity_result VARCHAR(50) NOT NULL,  -- e.g., success, failure
    activity_details TEXT,  -- Additional details or metadata about the activity (e.g., "Added user with ID 123")
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,  -- When the activity occurred
    ip_address VARCHAR(45),  -- IP address of the user (IPv4 or IPv6)
    error_message TEXT,  -- Optional: error message for failed activities
    user_agent TEXT  -- Optional: store user agent details (browser info)
);

-- Indexes for faster lookups
CREATE INDEX idx_user_activity_history_user_id ON user_activity_history(user_id);
CREATE INDEX idx_user_activity_history_activity_type ON user_activity_history(activity_type);
CREATE INDEX idx_user_activity_history_timestamp ON user_activity_history(timestamp);
CREATE INDEX idx_user_activity_history_ip_address ON user_activity_history(ip_address);