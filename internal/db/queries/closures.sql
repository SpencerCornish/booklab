-- name: ListClosures :many
SELECT * FROM closures ORDER BY start_date;

-- name: ListClosuresInRange :many
SELECT * FROM closures
WHERE start_date <= $2 AND end_date >= $1
ORDER BY start_date;

-- name: GetClosure :one
SELECT * FROM closures WHERE id = $1;

-- name: CreateClosure :one
INSERT INTO closures (start_date, end_date, all_day, start_time, end_time, reason)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpdateClosure :one
UPDATE closures SET
    start_date = $2,
    end_date   = $3,
    all_day    = $4,
    start_time = $5,
    end_time   = $6,
    reason     = $7
WHERE id = $1
RETURNING *;

-- name: DeleteClosure :exec
DELETE FROM closures WHERE id = $1;
