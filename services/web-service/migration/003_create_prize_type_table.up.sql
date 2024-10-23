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
    operator_id INT REFERENCES users(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- elligibility: number of prize of this type that can be given to the users
-- Index for fast lookup on name and type
CREATE INDEX idx_prize_type_name ON prize_type(name);
CREATE INDEX idx_prize_type_prize_category_id ON prize_type(prize_category_id);
CREATE INDEX idx_prize_type_prize_operator_id ON prize_type(operator_id);