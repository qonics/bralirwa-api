CREATE TABLE IF NOT EXISTS entries (
    id SERIAL PRIMARY KEY,
    customer_id INT REFERENCES customer(id),
    code_id INT REFERENCES codes(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Index for fast lookup on customer_id and code_id
CREATE INDEX idx_entries_customer_id ON entries(customer_id);
CREATE INDEX idx_entries_code_id ON entries(code_id);
