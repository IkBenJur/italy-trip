-- name: CreatePhoto :one
-- The id is supplied by the caller rather than defaulted, because the storage
-- keys (photos/{id}.jpg, thumbs/{id}.jpg) are written before the row exists.
INSERT INTO photos (
    id, event_id, uploaded_by, client_id, storage_key, thumb_key,
    content_type, width, height, size_bytes, taken_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: FindPhotoByClientId :one
SELECT * FROM photos
WHERE event_id = $1 AND client_id = $2;

-- name: ListPhotosByEvent :many
-- Ordered by capture time, not upload time: the offline retry queue means those
-- differ, and the album should read chronologically.
SELECT * FROM photos
WHERE event_id = $1
ORDER BY taken_at ASC, id ASC;

-- name: FindPhotoById :one
SELECT * FROM photos
WHERE id = $1;
