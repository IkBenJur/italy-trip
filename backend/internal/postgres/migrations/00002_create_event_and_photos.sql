-- +goose Up
CREATE TABLE IF NOT EXISTS events (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    starts_at  TIMESTAMPTZ NOT NULL,
    ends_at    TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The app is built around exactly one event. This makes "the singleton row" a
-- database guarantee rather than a convention the seeding code has to uphold,
-- and gives UpsertSingletonEvent a real conflict target to key off.
CREATE UNIQUE INDEX IF NOT EXISTS events_singleton_idx ON events ((TRUE));

CREATE TABLE IF NOT EXISTS photos (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id     UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    uploaded_by  UUID NOT NULL REFERENCES users(id),
    client_id    TEXT NOT NULL,
    storage_key  TEXT NOT NULL,
    thumb_key    TEXT NOT NULL,
    content_type TEXT NOT NULL,
    width        INTEGER NOT NULL,
    height       INTEGER NOT NULL,
    size_bytes   BIGINT NOT NULL,
    taken_at     TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (event_id, client_id)
);

CREATE INDEX IF NOT EXISTS photos_event_taken_idx ON photos (event_id, taken_at);

-- +goose Down
DROP TABLE photos;
DROP TABLE events;
