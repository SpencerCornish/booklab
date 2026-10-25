# Availability computation: bookable window, booked slots, closures.
# Default settings: 09:00-21:00 America/Denver → 12 hourly slots.
# 2030-03-15 is MDT (UTC-6): the second-Sunday DST switch is always ≤ Mar 14.

# Open future day: every slot available.
GET {{base_url}}/api/availability?date={{booking_date}}
HTTP 200
[Asserts]
jsonpath "$.date" == "2030-03-15"
jsonpath "$.is_closed" == false
jsonpath "$.slots" count == 12
jsonpath "$.slots[0].start" == "2030-03-15T09:00:00-06:00"
jsonpath "$.slots[11].start" == "2030-03-15T20:00:00-06:00"
jsonpath "$.slots[*].available" contains true

# The day-strip endpoint that drives the availability-aware date picker. It is
# anchored to "now" (window capped at 90 days), so assertions use near-term
# dates passed in by the runner.
GET {{base_url}}/api/availability/days?days=30
HTTP 200
[Asserts]
jsonpath "$.available_dates" contains "{{tomorrow}}"
jsonpath "$.available_dates" contains "{{day_after}}"

GET {{base_url}}/api/availability/days?days=nope
HTTP 400

# Book 10:00-12:00; the two covered slots must disappear.
POST {{base_url}}/api/bookings/prepare
{"email": "{{customer_email}}", "start_time": "{{booking_start}}", "end_time": "{{booking_end}}"}
HTTP 200
[Captures]
setup_intent_id: jsonpath "$.setup_intent_id"

POST {{base_url}}/api/bookings
{"setup_intent_id": "{{setup_intent_id}}", "name": "{{customer_name}}", "email": "{{customer_email}}", "start_time": "{{booking_start}}", "end_time": "{{booking_end}}"}
HTTP 201

GET {{base_url}}/api/availability?date={{booking_date}}
HTTP 200
[Asserts]
jsonpath "$.slots[1].available" == false
jsonpath "$.slots[2].available" == false
jsonpath "$.slots[3].available" == true

# Admin creates an all-day closure on a second date.
POST {{base_url}}/api/admin/login
{"username": "{{admin_user}}", "password": "{{admin_pass}}"}
HTTP 200

GET {{base_url}}/api/admin/closures
HTTP 200
[Captures]
csrf: header "Set-Cookie" regex "booklab_csrf=([^;]+)"
[Asserts]
jsonpath "$" count == 0

POST {{base_url}}/api/admin/closures
X-CSRF-Token: {{csrf}}
{"start_date": "{{other_date}}", "end_date": "{{other_date}}", "reason": "Painting weekend"}
HTTP 201

# Closed day: no slots, reason echoed back — and the day-strip no longer
# offers it.
GET {{base_url}}/api/availability?date={{other_date}}
HTTP 200
[Asserts]
jsonpath "$.is_closed" == true
jsonpath "$.closure_reason" == "Painting weekend"
jsonpath "$.slots" count == 0

GET {{base_url}}/api/availability/days?days=30
HTTP 200
[Asserts]
jsonpath "$.available_dates" not contains "2030-03-20"

# A near-term closure drops the day from the strip and closes the date itself.
POST {{base_url}}/api/admin/closures
X-CSRF-Token: {{csrf}}
{"start_date": "{{tomorrow}}", "end_date": "{{tomorrow}}", "reason": "Short notice"}
HTTP 201

GET {{base_url}}/api/availability?date={{tomorrow}}
HTTP 200
[Asserts]
jsonpath "$.is_closed" == true
jsonpath "$.closure_reason" == "Short notice"

GET {{base_url}}/api/availability/days?days=30
HTTP 200
[Asserts]
jsonpath "$.available_dates" not contains "{{tomorrow}}"

# Partial closure 12:00-14:00 on the open day: exactly slots 12:00 + 13:00 blocked.
POST {{base_url}}/api/admin/closures
X-CSRF-Token: {{csrf}}
{"start_date": "{{booking_date}}", "end_date": "{{booking_date}}", "all_day": false, "start_time": "12:00", "end_time": "14:00", "reason": "Maintenance"}
HTTP 201

GET {{base_url}}/api/availability?date={{booking_date}}
HTTP 200
[Asserts]
jsonpath "$.is_closed" == false
jsonpath "$.slots[3].available" == false
jsonpath "$.slots[4].available" == false
jsonpath "$.slots[5].available" == true

# Missing / malformed query params.
GET {{base_url}}/api/availability
HTTP 400

GET {{base_url}}/api/availability?date=not-a-date
HTTP 400
