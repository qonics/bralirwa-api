CREATE TABLE IF NOT EXISTS prize_type (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(100),
    value DECIMAL(10, 2),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Index for fast lookup on name and type
CREATE INDEX idx_prize_type_name ON prize_type(name);
CREATE INDEX idx_prize_type_type ON prize_type(type);