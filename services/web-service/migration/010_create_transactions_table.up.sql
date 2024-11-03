CREATE TABLE IF NOT EXISTS transaction (
    id SERIAL PRIMARY KEY,
    prize_id INT REFERENCES prize(id),
    amount DECIMAL(10, 2),
    phone VARCHAR(20),
    mno VARCHAR(20),
    initiated_by VARCHAR(50), -- SYSTEM or USER
    status VARCHAR(50), -- PENDING, SUCCESS, FAILED
    error_message TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
-- added extra columns from 017_alter_transaction_table.up.sql (customer_id, trx_id, ref_no, transaction_type)
-- transaction_type: 'DEBIT', 'CREDIT'
-- Index for fast lookup on prize_id and phone
CREATE INDEX idx_transaction_prize_id ON transaction(prize_id);
CREATE INDEX idx_transaction_phone ON transaction(phone);