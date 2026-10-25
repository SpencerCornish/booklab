# Admin authentication: sessions, CSRF double-submit, logout revocation, and
# login rate limiting (10 failures / 15 min per IP *and* per username).
#
# NOTE: this file must keep failed-login count deterministic — the rate-limit
# block is last and accounts for the single bad login above it. State is
# reset between suites anyway (login_attempts is truncated).

# Admin endpoints are protected without a session.
GET {{base_url}}/api/admin/bookings
HTTP 401

# Wrong password: 401, records one failed attempt.
POST {{base_url}}/api/admin/login
{"username": "{{admin_user}}", "password": "definitely-wrong"}
HTTP 401

# Correct credentials: session cookie set, JSON status ok.
POST {{base_url}}/api/admin/login
{"username": "{{admin_user}}", "password": "{{admin_pass}}"}
HTTP 200
[Asserts]
jsonpath "$.status" == "ok"
header "Set-Cookie" contains "HttpOnly"
header "Set-Cookie" contains "Secure"
header "Set-Cookie" contains "SameSite=Lax"

# The session cookie must not be exposed to JS: no document.cookie access.
# (The readable booklab_csrf cookie is set later, on the first admin request.)

# Authenticated request works and bootstraps the CSRF cookie.
GET {{base_url}}/api/admin/bookings
HTTP 200
[Captures]
csrf: header "Set-Cookie" regex "booklab_csrf=([^;]+)"
[Asserts]
jsonpath "$" count == 0

# First mutating request without an X-CSRF-Token header: 403 (the bootstrap
# booklab_csrf cookie is still set by middleware, captured above).
POST {{base_url}}/api/admin/closures
{"start_date": "{{other_date}}", "end_date": "{{other_date}}"}
HTTP 403

# Wrong token: 403.
POST {{base_url}}/api/admin/closures
X-CSRF-Token: deadbeef
{"start_date": "{{other_date}}", "end_date": "{{other_date}}"}
HTTP 403

# Correct token: 201.
POST {{base_url}}/api/admin/closures
X-CSRF-Token: {{csrf}}
{"start_date": "{{other_date}}", "end_date": "{{other_date}}", "reason": "auth-suite"}
HTTP 201
[Captures]
closure_id: jsonpath "$.id"

# Delete it again (also exercises DELETE + CSRF).
DELETE {{base_url}}/api/admin/closures/{{closure_id}}
X-CSRF-Token: {{csrf}}
HTTP 204

# Logout destroys the server-side session and clears the cookie.
POST {{base_url}}/api/admin/logout
HTTP 204

# Revoked session: subsequent admin calls are 401.
GET {{base_url}}/api/admin/bookings
HTTP 401

# ── Rate limiting ────────────────────────────────────────────────────────────
# One failure was recorded above. 9 more failures reach the cap (10 / 15 min);
# the next attempt — even with CORRECT credentials — is refused with 429.

POST {{base_url}}/api/admin/login
{"username": "{{admin_user}}", "password": "bad-pass-2"}
HTTP 401

POST {{base_url}}/api/admin/login
{"username": "{{admin_user}}", "password": "bad-pass-3"}
HTTP 401

POST {{base_url}}/api/admin/login
{"username": "{{admin_user}}", "password": "bad-pass-4"}
HTTP 401

POST {{base_url}}/api/admin/login
{"username": "{{admin_user}}", "password": "bad-pass-5"}
HTTP 401

POST {{base_url}}/api/admin/login
{"username": "{{admin_user}}", "password": "bad-pass-6"}
HTTP 401

POST {{base_url}}/api/admin/login
{"username": "{{admin_user}}", "password": "bad-pass-7"}
HTTP 401

POST {{base_url}}/api/admin/login
{"username": "{{admin_user}}", "password": "bad-pass-8"}
HTTP 401

POST {{base_url}}/api/admin/login
{"username": "{{admin_user}}", "password": "bad-pass-9"}
HTTP 401

POST {{base_url}}/api/admin/login
{"username": "{{admin_user}}", "password": "bad-pass-10"}
HTTP 401

# Cap reached: correct credentials still rejected.
POST {{base_url}}/api/admin/login
{"username": "{{admin_user}}", "password": "{{admin_pass}}"}
HTTP 429
