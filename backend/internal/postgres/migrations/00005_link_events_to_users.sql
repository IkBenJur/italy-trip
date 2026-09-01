-- +goose Up
-- user_id starts nullable so ADD COLUMN doesn't reject the existing rows;
-- the backfill migration fills it in for every pre-existing event (from the
-- seed user, the only account that could have created them) and then locks
-- it down with NOT NULL.
ALTER TABLE events ADD COLUMN user_id UUID REFERENCES users(id);
CREATE INDEX IF NOT EXISTS events_user_id_idx ON events (user_id);

-- A user may not have started an event yet, so this stays nullable.
ALTER TABLE users ADD COLUMN active_event_id UUID REFERENCES events(id);

-- +goose Down
ALTER TABLE users DROP COLUMN active_event_id;
DROP INDEX IF EXISTS events_user_id_idx;
ALTER TABLE events DROP COLUMN user_id;
