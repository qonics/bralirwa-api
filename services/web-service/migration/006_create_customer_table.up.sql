CREATE TABLE IF NOT EXISTS customer (
    id SERIAL PRIMARY KEY,
    names BYTEA NOT NULL,
    momo_names BYTEA NULL,
    phone BYTEA NOT NULL,
    phone_hash BYTEA NOT NULL UNIQUE,
    network_operator VARCHAR(20),
    locale VARCHAR(20) DEFAULT 'en',
    province  INT REFERENCES province(id),
    district  INT REFERENCES district(id),
    id_number VARCHAR(50),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Index for fast lookups on phone number
CREATE INDEX idx_customer_phone ON customer(phone_hash);
CREATE INDEX idx_customer_province ON customer(province);
CREATE INDEX idx_customer_district ON customer(district);