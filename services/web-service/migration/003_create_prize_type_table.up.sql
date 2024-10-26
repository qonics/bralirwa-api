CREATE TABLE IF NOT EXISTS prize_category (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    status VARCHAR(100),
    operator_id INT REFERENCES users(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- status (OKAY, DISABLED)
-- Index for fast lookup on name and type
CREATE INDEX idx_prize_category_name ON prize_category(name);
CREATE INDEX idx_prize_category_operator_id ON prize_category(operator_id);

CREATE TABLE IF NOT EXISTS prize_type (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    prize_category_id INT NOT NULL REFERENCES prize_category(id),
    elligibility INT,
    value DECIMAL(10, 2),
    status VARCHAR(255) DEFAULT 'OKAY',
    period VARCHAR(255) DEFAULT 'MONTHLY',
    expiry_date TIMESTAMP,
    distribution_type VARCHAR(255) DEFAULT 'momo',
    operator_id INT REFERENCES users(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- elligibility: number of prize of this type that can be given to the users
-- Index for fast lookup on name and type
CREATE INDEX idx_prize_type_name ON prize_type(name);
CREATE INDEX idx_prize_type_prize_category_id ON prize_type(prize_category_id);
CREATE INDEX idx_prize_type_prize_operator_id ON prize_type(operator_id);

CREATE TABLE IF NOT EXISTS prize_message (
    id SERIAL PRIMARY KEY,
    lang VARCHAR(10) NOT NULL,
    prize_type_id INT NOT NULL REFERENCES prize_type(id),
    status VARCHAR(255) DEFAULT 'OKAY',
    message VARCHAR(255) NOT NULL,
    operator_id INT REFERENCES users(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_prize_message_lang ON prize_message(lang);
CREATE INDEX idx_prize_message_prize_type_id ON prize_message(prize_type_id);
CREATE INDEX idx_prize_message_operator_id ON prize_message(operator_id);