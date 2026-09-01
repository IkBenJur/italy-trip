-- name: CreateEvent :one
INSERT INTO events (name, starts_at, ends_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetCurrentEvent :one
-- "Current" is the most recently started event; older events and their photos
-- stay in the table as history once a new one is started.
SELECT * FROM events
ORDER BY created_at DESC
LIMIT 1;

-- name: CountPhotos :one
SELECT COUNT(*) FROM photos
WHERE event_id = $1;
