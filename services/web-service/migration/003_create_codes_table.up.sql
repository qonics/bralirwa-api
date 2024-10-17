CREATE TABLE IF NOT EXISTS codes (
    id SERIAL PRIMARY KEY,
    code BYTEA NOT NULL,
    code_hash BYTEA NOT NULL,
    instant_prize BOOLEAN DEFAULT FALSE,
    status VARCHAR(50),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Index for fast lookup on code
CREATE INDEX idx_codes_code ON codes(code_hash);