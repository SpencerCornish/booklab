# Smoke-only check: the production stack wires its own Postgres, runs
# migrations on a fresh volume, bootstraps the admin, and serves admin APIs.
POST {{base_url}}/api/admin/login
{"username": "{{admin_user}}", "password": "{{admin_pass}}"}
HTTP 200

GET {{base_url}}/api/admin/bookings
HTTP 200
[Asserts]
jsonpath "$" count == 0
