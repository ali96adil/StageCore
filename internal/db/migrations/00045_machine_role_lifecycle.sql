-- +goose Up
ALTER TABLE machine_roles
ADD COLUMN retired INTEGER NOT NULL DEFAULT 0 CHECK(retired IN (0, 1));

ALTER TABLE machine_roles
ADD COLUMN retired_at_us INTEGER NULL;

-- +goose Down
ALTER TABLE machine_roles DROP COLUMN retired_at_us;
ALTER TABLE machine_roles DROP COLUMN retired;
