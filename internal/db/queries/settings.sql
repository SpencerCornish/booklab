-- name: GetSettings :one
SELECT * FROM settings WHERE id = 1;

-- name: UpdateSettings :one
UPDATE settings SET
    resource_name         = COALESCE(sqlc.narg('resource_name'), resource_name),
    hourly_rate_cents     = COALESCE(sqlc.narg('hourly_rate_cents'), hourly_rate_cents),
    currency              = COALESCE(sqlc.narg('currency'), currency),
    timezone              = COALESCE(sqlc.narg('timezone'), timezone),
    bookable_start        = COALESCE(sqlc.narg('bookable_start'), bookable_start),
    bookable_end          = COALESCE(sqlc.narg('bookable_end'), bookable_end),
    min_hours             = COALESCE(sqlc.narg('min_hours'), min_hours),
    max_hours             = COALESCE(sqlc.narg('max_hours'), max_hours),
    reminder_hours_before = COALESCE(sqlc.narg('reminder_hours_before'), reminder_hours_before)
WHERE id = 1
RETURNING *;
