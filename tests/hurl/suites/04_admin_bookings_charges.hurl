# Admin bookings list/filtering, PATCH updates, and the charge flow
# (fake Stripe: deterministic pi_fake_* IDs, computed amounts).
# Settings default: hourly_rate_cents = 1500.

POST {{base_url}}/api/admin/login
{"username": "{{admin_user}}", "password": "{{admin_pass}}"}
HTTP 200

GET {{base_url}}/api/admin/bookings
HTTP 200
[Captures]
csrf: header "Set-Cookie" regex "booklab_csrf=([^;]+)"
[Asserts]
jsonpath "$" count == 0

# Seed one booking via the public API (10:00-12:00 = 2h).
POST {{base_url}}/api/bookings/prepare
{"email": "{{customer_email}}", "start_time": "{{booking_start}}", "end_time": "{{booking_end}}"}
HTTP 200
[Captures]
setup_intent_id: jsonpath "$.setup_intent_id"

POST {{base_url}}/api/bookings
{"setup_intent_id": "{{setup_intent_id}}", "name": "{{customer_name}}", "email": "{{customer_email}}", "start_time": "{{booking_start}}", "end_time": "{{booking_end}}"}
HTTP 201
[Captures]
booking_id: jsonpath "$.booking.id"

# List + filters.
GET {{base_url}}/api/admin/bookings
HTTP 200
[Asserts]
jsonpath "$" count == 1
jsonpath "$[0].id" == {{booking_id}}
jsonpath "$[0].status" == "confirmed"
jsonpath "$[0].email" == "{{customer_email}}"
jsonpath "$[0].stripe_setup_intent_id" startsWith "seti_fake_"
jsonpath "$[0].stripe_payment_method_id" startsWith "pm_fake_"

GET {{base_url}}/api/admin/bookings?status=confirmed
HTTP 200
[Asserts]
jsonpath "$" count == 1

GET {{base_url}}/api/admin/bookings?status=cancelled
HTTP 200
[Asserts]
jsonpath "$" count == 0

GET {{base_url}}/api/admin/bookings?date={{booking_date}}
HTTP 200
[Asserts]
jsonpath "$" count == 1

GET {{base_url}}/api/admin/bookings?date=2030-03-16
HTTP 200
[Asserts]
jsonpath "$" count == 0

GET {{base_url}}/api/admin/bookings?status=bogus
HTTP 500

GET {{base_url}}/api/admin/bookings?date=not-a-date
HTTP 400

# Extend the booking to 13:00 (3h total).
PATCH {{base_url}}/api/admin/bookings/{{booking_id}}
X-CSRF-Token: {{csrf}}
{"end_time": "{{extended_end}}"}
HTTP 200
[Asserts]
jsonpath "$.end_time" == "2030-03-15T13:00:00-06:00"

# Mark completed (payment method on file → allowed).
PATCH {{base_url}}/api/admin/bookings/{{booking_id}}
X-CSRF-Token: {{csrf}}
{"status": "completed"}
HTTP 200
[Asserts]
jsonpath "$.status" == "completed"

# Charge with computed amount: 3h × 1500 = 4500.
POST {{base_url}}/api/admin/bookings/{{booking_id}}/charge
X-CSRF-Token: {{csrf}}
{}
HTTP 200
[Asserts]
jsonpath "$.status" == "charged"
jsonpath "$.amount_cents" == 4500
jsonpath "$.stripe_payment_intent_id" startsWith "pi_fake_"
jsonpath "$.stripe_receipt_url" contains "/receipts/"

# Second charge: 409 (idempotency).
POST {{base_url}}/api/admin/bookings/{{booking_id}}/charge
X-CSRF-Token: {{csrf}}
{}
HTTP 409

POST {{base_url}}/api/admin/bookings/{{booking_id}}/charge
X-CSRF-Token: {{csrf}}
{"amount_cents": 100}
HTTP 409

# Unknown booking: 404.
POST {{base_url}}/api/admin/bookings/99999/charge
X-CSRF-Token: {{csrf}}
{}
HTTP 404

# Amount validation needs a chargeable booking: seed + complete another one.
POST {{base_url}}/api/bookings/prepare
{"email": "{{customer_email}}", "start_time": "{{next_start}}", "end_time": "{{next_end}}"}
HTTP 200
[Captures]
setup_intent_2: jsonpath "$.setup_intent_id"

POST {{base_url}}/api/bookings
{"setup_intent_id": "{{setup_intent_2}}", "name": "Amount Booker", "email": "amount.functest@example.com", "start_time": "{{next_start}}", "end_time": "{{next_end}}"}
HTTP 201
[Captures]
booking_id_2: jsonpath "$.booking.id"

PATCH {{base_url}}/api/admin/bookings/{{booking_id_2}}
X-CSRF-Token: {{csrf}}
{"status": "completed"}
HTTP 200

POST {{base_url}}/api/admin/bookings/{{booking_id_2}}/charge
X-CSRF-Token: {{csrf}}
{"amount_cents": 0}
HTTP 400

POST {{base_url}}/api/admin/bookings/{{booking_id_2}}/charge
X-CSRF-Token: {{csrf}}
{"amount_cents": 25000}
HTTP 400

# Invalid amount reverts the claim, so the booking can still be charged.
POST {{base_url}}/api/admin/bookings/{{booking_id_2}}/charge
X-CSRF-Token: {{csrf}}
{"amount_cents": 1234}
HTTP 200
[Asserts]
jsonpath "$.status" == "charged"
jsonpath "$.amount_cents" == 1234

# Insights endpoint answers with a JSON object.
GET {{base_url}}/api/admin/insights
X-CSRF-Token: {{csrf}}
HTTP 200
[Asserts]
jsonpath "$" exists
