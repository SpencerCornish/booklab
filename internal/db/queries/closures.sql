-- name: ListClosures :many
SELECT * FROM closures ORDER BY start_date;

-- name: ListClosuresInRange :many
-- Calendar-date intersection: closures whose [start_date, end_date] overlap the
-- caller's day range. Casting the instant bounds to ::date keeps the comparison
-- in whole days, independent of the server/session timezone — comparing DATE
-- columns against raw timestamptz bounds misses same-day closures whenever the
-- app timezone is behind UTC (e.g. America/Denver).
SELECT * FROM closures
WHERE start_date <= $2::date AND end_date >= $1::date
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
