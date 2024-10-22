CREATE TABLE IF NOT EXISTS codes (
    id SERIAL PRIMARY KEY,
    code BYTEA NOT NULL,
    code_hash BYTEA NOT NULL UNIQUE,
    prize_type_id INT NULL DEFAULT NULL REFERENCES prize_type(id) ON DELETE RESTRICT,
    redeemed BOOLEAN DEFAULT FALSE,
    status VARCHAR(50),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Index for fast lookup on code
CREATE INDEX idx_codes_code ON codes(code_hash);
CREATE INDEX idx_codes_prize_type_id ON codes(prize_type_id);