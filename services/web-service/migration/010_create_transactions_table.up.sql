CREATE TABLE IF NOT EXISTS transaction (
    id SERIAL PRIMARY KEY,
    prize_id INT REFERENCES prize(id),
    amount DECIMAL(10, 2),
    phone VARCHAR(20),
    mno VARCHAR(20),
    initiated_by VARCHAR(50), -- System or User
    status VARCHAR(50),
    error_message TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Index for fast lookup on prize_id and phone
CREATE INDEX idx_transaction_prize_id ON transaction(prize_id);
CREATE INDEX idx_transaction_phone ON transaction(phone);