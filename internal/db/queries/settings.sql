-- name: GetSettings :one
SELECT * FROM settings WHERE id = 1;

-- name: UpdateSettings :one
UPDATE settings SET
    resource_name               = COALESCE(sqlc.narg('resource_name'), resource_name),
    hourly_rate_cents           = COALESCE(sqlc.narg('hourly_rate_cents'), hourly_rate_cents),
    currency                    = COALESCE(sqlc.narg('currency'), currency),
    timezone                    = COALESCE(sqlc.narg('timezone'), timezone),
    bookable_start              = COALESCE(sqlc.narg('bookable_start'), bookable_start),
    bookable_end                = COALESCE(sqlc.narg('bookable_end'), bookable_end),
    min_hours                   = COALESCE(sqlc.narg('min_hours'), min_hours),
    max_hours                   = COALESCE(sqlc.narg('max_hours'), max_hours),
    reminder_hours_before       = COALESCE(sqlc.narg('reminder_hours_before'), reminder_hours_before),
    new_customer_block_start    = CASE WHEN sqlc.arg('new_customer_block_set')::bool THEN sqlc.narg('new_customer_block_start') ELSE new_customer_block_start END,
    new_customer_block_end      = CASE WHEN sqlc.arg('new_customer_block_set')::bool THEN sqlc.narg('new_customer_block_end') ELSE new_customer_block_end END,
    new_customer_block_message  = COALESCE(sqlc.narg('new_customer_block_message'), new_customer_block_message)
WHERE id = 1
RETURNING *;
