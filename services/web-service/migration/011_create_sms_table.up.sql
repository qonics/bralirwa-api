CREATE TABLE IF NOT EXISTS sms (
    id SERIAL PRIMARY KEY,
    message TEXT NOT NULL,
    type VARCHAR(50), -- (registration, win, award failure, custom)
    customer_id INT REFERENCES customer(id),
    status VARCHAR(50),
    error_message VARCHAR(255),
    message_id VARCHAR(255),
    credit_count INT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Index for fast lookup on phone, type, and customer_id
CREATE INDEX idx_sms_type ON sms(type);
CREATE INDEX idx_sms_customer_id ON sms(customer_id);