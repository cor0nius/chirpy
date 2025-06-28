-- +goose Up
CREATE TABLE users (
    id INT PRIMARY KEY,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    email TEXT NOT NULL UNIQUE
);

-- +goose Down
DROP TABLE users;

--postgres://postgres:postgres@localhost:5432/chirpy