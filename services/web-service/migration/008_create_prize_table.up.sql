CREATE TABLE IF NOT EXISTS prize (
    id SERIAL PRIMARY KEY,
    customer_id INT REFERENCES customer(id),
    prize_type_id INT REFERENCES prize_type(id),
    prize_value DECIMAL(10, 2),
    rewarded BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Index for fast lookup on customer_id and prize_type_id
CREATE INDEX idx_prize_customer_id ON prize(customer_id);
CREATE INDEX idx_prize_prize_type_id ON prize(prize_type_id);