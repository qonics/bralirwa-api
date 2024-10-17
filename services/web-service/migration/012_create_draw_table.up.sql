CREATE TABLE IF NOT EXISTS draw (
    id SERIAL PRIMARY KEY,
    prize_type_id INT REFERENCES prize_type(id),
    code VARCHAR(255),
    customer_id INT REFERENCES customer(id),
    status VARCHAR(50), -- (confirmed, pending, closed)
    reason TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT unique_customer_prize UNIQUE (customer_id, prize_type_id),
    CONSTRAINT unique_code_prize UNIQUE (code, prize_type_id)
);

-- Index for fast lookup on code, customer_id, and status
CREATE INDEX idx_draw_code ON draw(code);
CREATE INDEX idx_draw_customer_id ON draw(customer_id);
CREATE INDEX idx_draw_status ON draw(status);