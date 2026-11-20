# Secret booking link: a token-holder books free sessions (no setup intent, no
# card) — including outside regular hours when allow_outside_hours is on —
# while conflicts, durations, and the charge path stay enforced. The DB is
# reset before this suite, so the settings row starts at defaults.

# ── Enable the link via admin settings ───────────────────────────────────────
POST {{base_url}}/api/admin/login
{"username": "{{admin_user}}", "password": "{{admin_pass}}"}
HTTP 200

GET {{base_url}}/api/admin/settings
HTTP 200
[Captures]
csrf: header "Set-Cookie" regex "booklab_csrf=([^;]+)"

# First enable bootstraps a 64-hex token and a /book/ URL.
PUT {{base_url}}/api/admin/settings
X-CSRF-Token: {{csrf}}
{"secret_link": {"enabled": true, "allow_outside_hours": true}, "notification_emails": "staff.secret@example.com"}
HTTP 200
[Captures]
secret_token: jsonpath "$.secret_link.token"
secret_url: jsonpath "$.secret_link.url"
[Asserts]
jsonpath "$.secret_link.enabled" == true
jsonpath "$.secret_link.allow_outside_hours" == true
jsonpath "$.secret_link.token" matches /^[0-9a-f]{64}$/
jsonpath "$.secret_link.url" contains "/book/"

# Unrelated settings updates must not disturb the token.
PUT {{base_url}}/api/admin/settings
X-CSRF-Token: {{csrf}}
{"resource_name": "The Space"}
HTTP 200
[Asserts]
jsonpath "$.secret_link.token" == "{{secret_token}}"

# ── Verify endpoint: uniform 200 with a valid flag ───────────────────────────
GET {{base_url}}/api/secret-link/verify?token={{secret_token}}
HTTP 200
[Asserts]
jsonpath "$.valid" == true
jsonpath "$.resource_name" == "The Space"
jsonpath "$.any_hours" == true

GET {{base_url}}/api/secret-link/verify?token=0000000000000000000000000000000000000000000000000000000000000000
HTTP 200
[Asserts]
jsonpath "$.valid" == false

# ── Privacy pin: public settings never mention the link ──────────────────────
GET {{base_url}}/api/settings/public
HTTP 200
[Asserts]
body not contains "secret_link"

# ── Off-hours are reserved for the link ──────────────────────────────────────
# 02:00-04:00 America/Denver sits outside the 09:00-21:00 default window.
# The standard paid flow rejects it at prepare time...
POST {{base_url}}/api/bookings/prepare
{"email": "offhours@example.com", "start_time": "2030-03-15T08:00:00Z", "end_time": "2030-03-15T10:00:00Z"}
HTTP 400

# ...but a valid token books it free, with no setup intent at all.
POST {{base_url}}/api/bookings
X-Secret-Token: {{secret_token}}
{"name": "Studio Artist", "email": "artist.secret@example.com", "start_time": "2030-03-15T08:00:00Z", "end_time": "2030-03-15T10:00:00Z"}
HTTP 201
[Captures]
free_booking_id: jsonpath "$.booking.id"
free_booking_token: jsonpath "$.booking.cancel_token"
[Asserts]
jsonpath "$.booking.status" == "confirmed"
jsonpath "$.booking.name" == "Studio Artist"

# A network retry (no idempotency key on the free flow) conflicts.
POST {{base_url}}/api/bookings
X-Secret-Token: {{secret_token}}
{"name": "Studio Artist", "email": "artist.secret@example.com", "start_time": "2030-03-15T08:00:00Z", "end_time": "2030-03-15T10:00:00Z"}
HTTP 409

# An overlapping slot is still rejected for link holders.
POST {{base_url}}/api/bookings
X-Secret-Token: {{secret_token}}
{"name": "Overlap", "email": "overlap.secret@example.com", "start_time": "2030-03-15T09:00:00Z", "end_time": "2030-03-15T11:00:00Z"}
HTTP 409

# A bad token is just the paid flow without a card: 400, never a free row.
POST {{base_url}}/api/bookings
X-Secret-Token: wrongwrongwrongwrongwrongwrongwrongwrongwrongwrongwrongwrongwr
{"name": "Sneaky", "email": "sneaky@example.com", "start_time": "2030-03-15T12:00:00Z", "end_time": "2030-03-15T14:00:00Z"}
HTTP 400
[Asserts]
jsonpath "$.error" == "card not confirmed"

# ── Free bookings in the admin list, and the charge backstop ─────────────────
GET {{base_url}}/api/admin/bookings
X-CSRF-Token: {{csrf}}
HTTP 200
[Asserts]
jsonpath "$[0].via_secret_link" == true
jsonpath "$[0].stripe_payment_method_id" == null

# Marking a free booking done needs no payment method on file.
PATCH {{base_url}}/api/admin/bookings/{{free_booking_id}}
X-CSRF-Token: {{csrf}}
{"status": "completed"}
HTTP 200
[Asserts]
jsonpath "$.status" == "completed"

# Charging it is a 400 with a fixed message — never a 500 or a panic.
POST {{base_url}}/api/admin/bookings/{{free_booking_id}}/charge
X-CSRF-Token: {{csrf}}
{"amount_cents": 2000}
HTTP 400
[Asserts]
jsonpath "$.error" == "free studio booking cannot be charged"

# ── Mailpit: free-booking email copy ─────────────────────────────────────────
GET {{mailpit_url}}/api/v1/search?query=to:artist.secret@example.com
[Options]
retry: 30
retry-interval: 500ms
HTTP 200
[Captures]
confirmation_id: jsonpath "$.messages[0].ID"
[Asserts]
jsonpath "$.messages[0].Subject" == "Booking Confirmed – The Space"

GET {{mailpit_url}}/api/v1/message/{{confirmation_id}}
HTTP 200
[Asserts]
jsonpath "$.HTML" contains "Studio booking - no payment required"
jsonpath "$.HTML" contains "/cancel/{{free_booking_token}}"

# Search targets the new-booking email by subject: the completion notice (also
# addressed to staff) lands later in the inbox.
GET {{mailpit_url}}/api/v1/search?query=to:staff.secret%40example.com%20subject%3A%22New%20Booking%22
[Options]
retry: 30
retry-interval: 500ms
HTTP 200
[Captures]
staff_id: jsonpath "$.messages[0].ID"
[Asserts]
jsonpath "$.messages[0].Subject" == "New Booking – The Space"

GET {{mailpit_url}}/api/v1/message/{{staff_id}}
HTTP 200
[Asserts]
jsonpath "$.HTML" contains "[Free Studio Booking]"
