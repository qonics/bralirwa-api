
ALTER TABLE transaction ADD COLUMN customer_id INT REFERENCES customer(id) ON DELETE RESTRICT;
ALTER TABLE transaction ADD COLUMN trx_id VARCHAR(50);
ALTER TABLE transaction ADD COLUMN ref_no VARCHAR(50);
ALTER TABLE transaction ADD COLUMN transaction_type VARCHAR(50);