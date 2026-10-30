-- +goose Up
CREATE TABLE expenses (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL,
    amount NUMERIC(12, 2) NOT NULL,
    category TEXT NOT NULL,
    spent_at DATE NOT NULL,
    comment TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT expenses_amount_positive CHECK (amount > 0)
);

CREATE INDEX expenses_user_id_spent_at_idx ON expenses (user_id, spent_at DESC);

-- +goose Down
DROP TABLE IF EXISTS expenses;
