-- name: UpsertSingletonEvent :one
-- Keyed on the events_singleton_idx expression index, so a second boot updates
-- the existing row in place instead of creating a second event.
INSERT INTO events (name, starts_at, ends_at)
VALUES ($1, $2, $3)
ON CONFLICT ((TRUE)) DO UPDATE
SET name = EXCLUDED.name,
    starts_at = EXCLUDED.starts_at,
    ends_at = EXCLUDED.ends_at,
    updated_at = now()
RETURNING *;

-- name: GetCurrentEvent :one
SELECT * FROM events
ORDER BY created_at
LIMIT 1;

-- name: CountPhotos :one
SELECT COUNT(*) FROM photos
WHERE event_id = $1;
