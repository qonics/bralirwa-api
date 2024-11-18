CREATE TABLE IF NOT EXISTS transaction_records (
    id SERIAL PRIMARY KEY,
    transaction_id INT REFERENCES transaction(id),
    trx_id VARCHAR(50),
    ref_no VARCHAR(50),
    amount DECIMAL(10, 2),
    transaction_type VARCHAR(50), -- DEBIT, CREDIT
    phone VARCHAR(20),
    mno VARCHAR(20),
    status VARCHAR(50), -- PENDING, SUCCESS, FAILED
    error_message TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_transaction_records_transaction_id ON transaction_records(transaction_id);
CREATE INDEX idx_transaction_records_trx_id ON transaction_records(trx_id);
CREATE INDEX idx_transaction_records_ref_no ON transaction_records(ref_no);
CREATE INDEX idx_transaction_records_phone ON transaction_records(phone);