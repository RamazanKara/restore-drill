-- Integration test fixture: MySQL seed data
CREATE TABLE users (
    id INT AUTO_INCREMENT PRIMARY KEY,
    email VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE orders (
    id INT AUTO_INCREMENT PRIMARY KEY,
    user_id INT,
    total DECIMAL(10,2),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
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
