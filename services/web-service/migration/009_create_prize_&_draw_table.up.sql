CREATE TABLE IF NOT EXISTS draw (
    id SERIAL PRIMARY KEY,
    prize_type_id INT REFERENCES prize_type(id),
    code VARCHAR(255),
    entry_id INT REFERENCES entries(id),
    customer_id INT REFERENCES customer(id),
    status VARCHAR(50), -- (confirmed, pending, closed)
    reason TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    operator_id INT REFERENCES users(id),
    CONSTRAINT unique_customer_prize UNIQUE (customer_id, prize_type_id),
    CONSTRAINT unique_code_prize UNIQUE (code, prize_type_id)
);

-- Index for fast lookup on code, customer_id, and status
CREATE INDEX idx_draw_code ON draw(code);
CREATE INDEX idx_draw_customer_id ON draw(customer_id);
CREATE INDEX idx_draw_entry_id ON draw(entry_id);
CREATE INDEX idx_draw_operator_id ON draw(operator_id);
CREATE INDEX idx_draw_status ON draw(status);

CREATE TABLE IF NOT EXISTS prize (
    id SERIAL PRIMARY KEY,
    entry_id INT REFERENCES entries(id),
    prize_type_id INT REFERENCES prize_type(id),
    prize_value DECIMAL(10, 2),
    code VARCHAR(255),
    draw_id INT REFERENCES draw(id) NULL DEFAULT NULL,
    rewarded BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_prize_entry_id ON prize(entry_id);
CREATE INDEX idx_prize_prize_type_id ON prize(prize_type_id);
CREATE INDEX idx_prize_draw_id ON prize(draw_id);