-- name: CreateUser :one
INSERT INTO users (email, password_hash, name, role)
VALUES ($1, $2, $3, $4)
RETURNING id;

-- name: GetUserByEmail :one
SELECT id, email::text AS email, password_hash, name, role
FROM users
WHERE email = $1;

-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, expires_at, user_agent, ip)
VALUES ($1, $2, $3, $4, $5);

-- name: GetSessionUser :one
SELECT u.id, u.email::text AS email, u.name, u.role
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1 AND s.expires_at > now();

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = $1;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE expires_at <= now();
