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
    notification_emails         = COALESCE(sqlc.narg('notification_emails'), notification_emails),
    auto_charge_delay_minutes   = COALESCE(sqlc.narg('auto_charge_delay_minutes'), auto_charge_delay_minutes),
    referral_sources            = COALESCE(sqlc.narg('referral_sources'), referral_sources),
    terms_content               = COALESCE(sqlc.narg('terms_content'), terms_content),
    privacy_content             = COALESCE(sqlc.narg('privacy_content'), privacy_content),
    min_lead_hours              = COALESCE(sqlc.narg('min_lead_hours'), min_lead_hours),
    new_customer_block_start    = CASE WHEN sqlc.arg('new_customer_block_set')::bool THEN sqlc.narg('new_customer_block_start') ELSE new_customer_block_start END,
    new_customer_block_end      = CASE WHEN sqlc.arg('new_customer_block_set')::bool THEN sqlc.narg('new_customer_block_end') ELSE new_customer_block_end END,
    new_customer_block_message  = COALESCE(sqlc.narg('new_customer_block_message'), new_customer_block_message),
    new_user_gate_enabled       = COALESCE(sqlc.narg('new_user_gate_enabled'), new_user_gate_enabled),
    new_user_gate_question      = COALESCE(sqlc.narg('new_user_gate_question'), new_user_gate_question),
    new_user_gate_schedule      = COALESCE(sqlc.narg('new_user_gate_schedule'), new_user_gate_schedule),
    secret_link_enabled         = COALESCE(sqlc.narg('secret_link_enabled'), secret_link_enabled),
    secret_link_any_hours       = COALESCE(sqlc.narg('secret_link_any_hours'), secret_link_any_hours)
WHERE id = 1
RETURNING *;

-- name: SetSecretLinkToken :one
-- Only called by the admin rotate endpoint and the enable-time bootstrap, so
-- the previous token is unconditionally replaced: rotation revokes the old
-- link immediately. An empty argument clears the token.
UPDATE settings SET secret_link_token = NULLIF($1, '') WHERE id = 1 RETURNING *;
