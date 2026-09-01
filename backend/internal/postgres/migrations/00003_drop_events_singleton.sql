-- +goose Up
-- Events are no longer a singleton: starting a new event inserts a new row
-- and keeps the old one (and its photos) around as history. "Current" is now
-- whichever event was created most recently, not "the only row".
DROP INDEX IF EXISTS events_singleton_idx;

-- +goose Down
CREATE UNIQUE INDEX IF NOT EXISTS events_singleton_idx ON events ((TRUE));
