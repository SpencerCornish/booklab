# Admin settings, users, and closures management. State is reset after this
# suite (settings row restored to defaults), so mutations here stay local.

POST {{base_url}}/api/admin/login
{"username": "{{admin_user}}", "password": "{{admin_pass}}"}
HTTP 200

GET {{base_url}}/api/admin/settings
HTTP 200
[Captures]
csrf: header "Set-Cookie" regex "booklab_csrf=([^;]+)"
[Asserts]
jsonpath "$.resource_name" == "The Space"
jsonpath "$.hourly_rate_cents" == 1500

# ── Settings ─────────────────────────────────────────────────────────────────
PUT {{base_url}}/api/admin/settings
X-CSRF-Token: {{csrf}}
{"resource_name": "Functional Test Space", "min_hours": 2, "notification_emails": "staff.functest@example.com"}
HTTP 200
[Asserts]
jsonpath "$.resource_name" == "Functional Test Space"
jsonpath "$.min_hours" == 2
jsonpath "$.notification_emails" == "staff.functest@example.com"

# Public settings must reflect admin edits.
GET {{base_url}}/api/settings/public
HTTP 200
[Asserts]
jsonpath "$.resource_name" == "Functional Test Space"
jsonpath "$.min_hours" == 2

# Invalid bookable hours format: 400.
PUT {{base_url}}/api/admin/settings
X-CSRF-Token: {{csrf}}
{"bookable_start": "9am"}
HTTP 400

# Negative lead hours: 400.
PUT {{base_url}}/api/admin/settings
X-CSRF-Token: {{csrf}}
{"min_lead_hours": -1}
HTTP 400

# New-customer restriction with end before start: 400.
PUT {{base_url}}/api/admin/settings
X-CSRF-Token: {{csrf}}
{"new_customer_restriction": {"start_date": "2030-06-10", "end_date": "2030-06-01"}}
HTTP 400

# Valid restriction round-trips, then clears again with null.
PUT {{base_url}}/api/admin/settings
X-CSRF-Token: {{csrf}}
{"new_customer_restriction": {"start_date": "2030-06-01", "end_date": "2030-06-10", "message": "Members only"}}
HTTP 200
[Asserts]
jsonpath "$.new_customer_restriction.start_date" == "2030-06-01"
jsonpath "$.new_customer_restriction.message" == "Members only"

GET {{base_url}}/api/settings/public
HTTP 200
[Asserts]
jsonpath "$.new_customer_restriction.message" == "Members only"

PUT {{base_url}}/api/admin/settings
X-CSRF-Token: {{csrf}}
{"new_customer_restriction": null}
HTTP 200
[Asserts]
jsonpath "$.new_customer_restriction" == null

# ── Users ────────────────────────────────────────────────────────────────────
GET {{base_url}}/api/admin/users
HTTP 200
[Asserts]
jsonpath "$.users" count == 1
jsonpath "$.current_username" == "{{admin_user}}"

POST {{base_url}}/api/admin/users
X-CSRF-Token: {{csrf}}
{"username": "secondadmin-{{run_id}}", "password": "second-pass-123"}
HTTP 201
[Asserts]
jsonpath "$.username" == "secondadmin-{{run_id}}"

POST {{base_url}}/api/admin/users
X-CSRF-Token: {{csrf}}
{"username": "secondadmin-{{run_id}}", "password": "second-pass-123"}
HTTP 409

POST {{base_url}}/api/admin/users
X-CSRF-Token: {{csrf}}
{"username": "thirdadmin-{{run_id}}", "password": "short"}
HTTP 400

# Still authenticated as admin: cannot delete self or unknown users.
DELETE {{base_url}}/api/admin/users/{{admin_user}}
X-CSRF-Token: {{csrf}}
HTTP 400

DELETE {{base_url}}/api/admin/users/nobody
X-CSRF-Token: {{csrf}}
HTTP 404

# Log in as the new admin (cookie store switches the active session).
POST {{base_url}}/api/admin/login
{"username": "secondadmin-{{run_id}}", "password": "second-pass-123"}
HTTP 200

GET {{base_url}}/api/admin/users
HTTP 200
[Asserts]
jsonpath "$.current_username" == "secondadmin-{{run_id}}"

# Back to the original admin, then remove the temporary one.
POST {{base_url}}/api/admin/login
{"username": "{{admin_user}}", "password": "{{admin_pass}}"}
HTTP 200

DELETE {{base_url}}/api/admin/users/secondadmin-{{run_id}}
X-CSRF-Token: {{csrf}}
HTTP 204

GET {{base_url}}/api/admin/users
HTTP 200
[Asserts]
jsonpath "$.users" count == 1

# ── Password change (restores original at the end) ──────────────────────────
PUT {{base_url}}/api/admin/users/me/password
X-CSRF-Token: {{csrf}}
{"current_password": "{{admin_pass}}", "new_password": "rotated-pass-99"}
HTTP 204

# Server issues a fresh session; old sessions are revoked.
GET {{base_url}}/api/admin/bookings
HTTP 200

# Old password no longer works.
POST {{base_url}}/api/admin/login
{"username": "{{admin_user}}", "password": "{{admin_pass}}"}
HTTP 401

# New password works.
POST {{base_url}}/api/admin/login
{"username": "{{admin_user}}", "password": "rotated-pass-99"}
HTTP 200

PUT {{base_url}}/api/admin/users/me/password
X-CSRF-Token: {{csrf}}
{"current_password": "rotated-pass-99", "new_password": "{{admin_pass}}"}
HTTP 204

POST {{base_url}}/api/admin/login
{"username": "{{admin_user}}", "password": "{{admin_pass}}"}
HTTP 200

# ── Closures CRUD ────────────────────────────────────────────────────────────
GET {{base_url}}/api/admin/closures
HTTP 200
[Asserts]
jsonpath "$" count == 0

POST {{base_url}}/api/admin/closures
X-CSRF-Token: {{csrf}}
{"start_date": "2030-07-01", "end_date": "2030-07-03", "reason": "Deep clean"}
HTTP 201
[Captures]
closure_id: jsonpath "$.id"
[Asserts]
jsonpath "$.all_day" == true
jsonpath "$.start_time" == null

POST {{base_url}}/api/admin/closures
X-CSRF-Token: {{csrf}}
{"start_date": "2030-07-05", "end_date": "2030-07-01"}
HTTP 400

POST {{base_url}}/api/admin/closures
X-CSRF-Token: {{csrf}}
{"start_date": "2030-07-05", "end_date": "2030-07-06", "all_day": false}
HTTP 400

GET {{base_url}}/api/admin/closures
HTTP 200
[Asserts]
jsonpath "$" count == 1
jsonpath "$[0].reason" == "Deep clean"

PUT {{base_url}}/api/admin/closures/{{closure_id}}
X-CSRF-Token: {{csrf}}
{"start_date": "2030-07-02", "end_date": "2030-07-04", "reason": "Updated clean"}
HTTP 200
[Asserts]
jsonpath "$.reason" == "Updated clean"

DELETE {{base_url}}/api/admin/closures/{{closure_id}}
X-CSRF-Token: {{csrf}}
HTTP 204

GET {{base_url}}/api/admin/closures
HTTP 200
[Asserts]
jsonpath "$" count == 0
