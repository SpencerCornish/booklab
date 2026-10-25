# Public booking lifecycle against STRIPE_FAKE=1:
# prepare → create → view → replay idempotency → conflicts → cancel → rebook.
# Every booking below is on 2030-03-15 within 09:00-21:00 America/Denver.

# Prepare returns a fake SetupIntent (proves STRIPE_FAKE is active).
POST {{base_url}}/api/bookings/prepare
{"email": "{{customer_email}}", "start_time": "{{booking_start}}", "end_time": "{{booking_end}}"}
HTTP 200
[Captures]
setup_intent_id: jsonpath "$.setup_intent_id"
[Asserts]
jsonpath "$.setup_intent_id" startsWith "seti_fake_"
jsonpath "$.setup_intent_client_secret" startsWith "cs_fake_"

# Create booking: 201 with a confirmed booking and a cancel token.
POST {{base_url}}/api/bookings
{"setup_intent_id": "{{setup_intent_id}}", "name": "{{customer_name}}", "email": "{{customer_email}}", "start_time": "{{booking_start}}", "end_time": "{{booking_end}}"}
HTTP 201
[Captures]
booking_token: jsonpath "$.booking.cancel_token"
[Asserts]
jsonpath "$.booking.status" == "confirmed"
jsonpath "$.booking.name" == "{{customer_name}}"
jsonpath "$.booking.email" == "{{customer_email}}"
jsonpath "$.booking.start_time" == "2030-03-15T10:00:00-06:00"
jsonpath "$.booking.end_time" == "2030-03-15T12:00:00-06:00"

# Idempotent replay: same setup_intent_id returns the same booking.
POST {{base_url}}/api/bookings
{"setup_intent_id": "{{setup_intent_id}}", "name": "{{customer_name}}", "email": "{{customer_email}}", "start_time": "{{booking_start}}", "end_time": "{{booking_end}}"}
HTTP 201
[Asserts]
jsonpath "$.booking.cancel_token" == "{{booking_token}}"

# Prepare for an already-taken slot is rejected up front.
POST {{base_url}}/api/bookings/prepare
{"email": "{{customer_email}}", "start_time": "{{booking_start}}", "end_time": "{{booking_end}}"}
HTTP 409

# A merely-overlapping window is rejected at prepare time as well.
POST {{base_url}}/api/bookings/prepare
{"email": "{{customer_email}}", "start_time": "2030-03-15T11:00:00-06:00", "end_time": "2030-03-15T13:00:00-06:00"}
HTTP 409

# Prepare a free slot, then try to spend that intent on the conflicting one.
POST {{base_url}}/api/bookings/prepare
{"email": "{{customer_email}}", "start_time": "{{next_start}}", "end_time": "{{next_end}}"}
HTTP 200
[Captures]
setup_intent_conflict: jsonpath "$.setup_intent_id"

POST {{base_url}}/api/bookings
{"setup_intent_id": "{{setup_intent_conflict}}", "name": "Second Booker", "email": "second.functest@example.com", "start_time": "{{booking_start}}", "end_time": "{{booking_end}}"}
HTTP 409

# The same intent books its own (adjacent) slot: 12:00-13:00.
POST {{base_url}}/api/bookings
{"setup_intent_id": "{{setup_intent_conflict}}", "name": "Adjacent Booker", "email": "adjacent.functest@example.com", "start_time": "{{booking_end}}", "end_time": "{{next_start}}"}
HTTP 201
[Asserts]
jsonpath "$.booking.start_time" == "2030-03-15T12:00:00-06:00"

# Validation errors.
POST {{base_url}}/api/bookings/prepare
{"email": "", "start_time": "{{booking_start}}", "end_time": "{{booking_end}}"}
HTTP 400

POST {{base_url}}/api/bookings/prepare
{"email": "{{customer_email}}", "start_time": "{{booking_end}}", "end_time": "{{booking_start}}"}
HTTP 400

POST {{base_url}}/api/bookings/prepare
{"email": "{{customer_email}}", "start_time": "2019-01-01T10:00:00-06:00", "end_time": "2019-01-01T12:00:00-06:00"}
HTTP 400

POST {{base_url}}/api/bookings/prepare
{"email": "{{customer_email}}", "start_time": "{{booking_start}}", "end_time": "2030-03-15T21:00:00-06:00"}
HTTP 400

# Unknown setup intent (fake rejects non-fake IDs): card not confirmed → 400.
POST {{base_url}}/api/bookings
{"setup_intent_id": "seti_does_not_exist", "name": "X", "email": "x.functest@example.com", "start_time": "{{next_start}}", "end_time": "{{next_end}}"}
HTTP 400

POST {{base_url}}/api/bookings
{"setup_intent_id": "", "name": "", "email": "", "start_time": "{{next_start}}", "end_time": "{{next_end}}"}
HTTP 400

POST {{base_url}}/api/bookings
{}
HTTP 400

# View by token.
GET {{base_url}}/api/bookings/{{booking_token}}
HTTP 200
[Asserts]
jsonpath "$.status" == "confirmed"

GET {{base_url}}/api/bookings/not-a-uuid
HTTP 400

GET {{base_url}}/api/bookings/{{unknown_token}}
HTTP 404

# Cancel, then the slot is free again.
POST {{base_url}}/api/bookings/{{booking_token}}/cancel
HTTP 200
[Asserts]
jsonpath "$.status" == "cancelled"

# Double cancel: 409.
POST {{base_url}}/api/bookings/{{booking_token}}/cancel
HTTP 409

POST {{base_url}}/api/bookings/{{unknown_token}}/cancel
HTTP 409

# Rebook the freed slot.
POST {{base_url}}/api/bookings/prepare
{"email": "{{customer_email}}", "start_time": "{{booking_start}}", "end_time": "{{booking_end}}"}
HTTP 200
[Captures]
setup_intent_rebook: jsonpath "$.setup_intent_id"

POST {{base_url}}/api/bookings
{"setup_intent_id": "{{setup_intent_rebook}}", "name": "Rebooker", "email": "rebook.functest@example.com", "start_time": "{{booking_start}}", "end_time": "{{booking_end}}"}
HTTP 201
[Captures]
rebook_token: jsonpath "$.booking.cancel_token"

POST {{base_url}}/api/bookings/{{rebook_token}}/cancel
HTTP 200
