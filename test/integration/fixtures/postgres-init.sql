-- Integration test fixture: PostgreSQL seed data
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE orders (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id),
    total DECIMAL(10,2),
    created_at TIMESTAMP DEFAULT NOW()
);

INSERT INTO users (email) VALUES
    ('alice@example.com'),
    ('bob@example.com'),
    ('carol@example.com');

INSERT INTO orders (user_id, total) VALUES
    (1, 99.99),
    (2, 49.50),
    (3, 150.00),
    (1, 25.00);
