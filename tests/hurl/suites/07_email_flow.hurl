# Email delivery through Mailpit (real SMTP on the test stack):
# confirmation after create, cancellation after cancel, receipt after charge.
# Uses a dedicated recipient so earlier suites' mail (if the clear failed) can
# never collide — this address only exists in this suite.
#
# Note: only a confirmed (not yet charged) booking can be cancelled via its
# token, so the receipt flow uses a second booking.

# Booking A: create → confirmation email.
POST {{base_url}}/api/bookings/prepare
{"email": "{{email_flow_email}}", "start_time": "{{booking_start}}", "end_time": "{{booking_end}}"}
HTTP 200
[Captures]
setup_intent_id: jsonpath "$.setup_intent_id"

POST {{base_url}}/api/bookings
{"setup_intent_id": "{{setup_intent_id}}", "name": "{{email_flow_name}}", "email": "{{email_flow_email}}", "start_time": "{{booking_start}}", "end_time": "{{booking_end}}"}
HTTP 201
[Captures]
booking_token: jsonpath "$.booking.cancel_token"

# Confirmation email arrives (SMTP send is async → retry until it does).
GET {{mailpit_url}}/api/v1/search?query=to:{{email_flow_email}}
[Options]
retry: 30
retry-interval: 500ms
HTTP 200
[Captures]
confirmation_id: jsonpath "$.messages[0].ID"
[Asserts]
jsonpath "$.messages[0].Subject" == "Booking Confirmed – The Space"
jsonpath "$.total" >= 1

# The confirmation body carries view + cancel links for THIS booking.
GET {{mailpit_url}}/api/v1/message/{{confirmation_id}}
HTTP 200
[Asserts]
jsonpath "$.HTML" contains "/booking/{{booking_token}}"
jsonpath "$.HTML" contains "/cancel/{{booking_token}}"

# Cancel → cancellation email.
POST {{base_url}}/api/bookings/{{booking_token}}/cancel
HTTP 200

GET {{mailpit_url}}/api/v1/search?query=to:{{email_flow_email}}
[Options]
retry: 30
retry-interval: 500ms
HTTP 200
[Captures]
cancellation_id: jsonpath "$.messages[0].ID"
[Asserts]
jsonpath "$.messages[0].Subject" == "Booking Cancelled – The Space"

# Booking B: admin completes + charges → receipt email.
POST {{base_url}}/api/bookings/prepare
{"email": "{{email_flow_email}}", "start_time": "{{next_start}}", "end_time": "{{next_end}}"}
HTTP 200
[Captures]
setup_intent_b: jsonpath "$.setup_intent_id"

POST {{base_url}}/api/bookings
{"setup_intent_id": "{{setup_intent_b}}", "name": "{{email_flow_name}}", "email": "{{email_flow_email}}", "start_time": "{{next_start}}", "end_time": "{{next_end}}"}
HTTP 201

POST {{base_url}}/api/admin/login
{"username": "{{admin_user}}", "password": "{{admin_pass}}"}
HTTP 200

GET {{base_url}}/api/admin/bookings?status=confirmed
HTTP 200
[Captures]
csrf: header "Set-Cookie" regex "booklab_csrf=([^;]+)"
booking_id_b: jsonpath "$[0].id"
[Asserts]
jsonpath "$" count == 1

PATCH {{base_url}}/api/admin/bookings/{{booking_id_b}}
X-CSRF-Token: {{csrf}}
{"status": "completed"}
HTTP 200

POST {{base_url}}/api/admin/bookings/{{booking_id_b}}/charge
X-CSRF-Token: {{csrf}}
{"amount_cents": 3000}
HTTP 200

GET {{mailpit_url}}/api/v1/search?query=to:{{email_flow_email}}
[Options]
retry: 30
retry-interval: 500ms
HTTP 200
[Captures]
receipt_id: jsonpath "$.messages[0].ID"
[Asserts]
jsonpath "$.messages[0].Subject" == "Receipt – The Space"

GET {{mailpit_url}}/api/v1/message/{{receipt_id}}
HTTP 200
[Asserts]
jsonpath "$.HTML" contains "30.00"

# Exactly four messages for this recipient, in this suite:
# A-confirmation, A-cancellation, B-confirmation, B-receipt.
GET {{mailpit_url}}/api/v1/search?query=to:{{email_flow_email}}
HTTP 200
[Asserts]
jsonpath "$.total" == 4
