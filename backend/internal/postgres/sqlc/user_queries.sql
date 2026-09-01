-- name: CreateUser :one
INSERT INTO users (email, password_hash)
VALUES ($1, $2)
RETURNING *;

-- name: FindUserById :one
SELECT * FROM users
WHERE id = $1;

-- name: FindUserByEmail :one
SELECT * FROM users
WHERE email = $1;

-- name: ListUsers :many
SELECT * FROM users
ORDER BY created_at DESC;

-- name: SetActiveEvent :exec
UPDATE users
SET active_event_id = $2, updated_at = now()
WHERE id = $1;
