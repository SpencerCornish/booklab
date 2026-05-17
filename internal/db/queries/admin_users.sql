-- name: GetAdminByUsername :one
SELECT * FROM admin_users WHERE username = $1;

-- name: CreateAdminUser :one
INSERT INTO admin_users (username, password_hash)
VALUES ($1, $2)
RETURNING *;
