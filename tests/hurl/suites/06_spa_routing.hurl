# SPA serving from the embedded filesystem of the real binary: index fallback,
# static assets, probe-path 404s, and unknown-path 404s.

# Root serves the built index.html.
GET {{base_url}}/
HTTP 200
[Asserts]
header "Content-Type" contains "text/html"
body contains "<!doctype html>"

# The bundle name is content-hashed per build; capture it and fetch it.
GET {{base_url}}/
HTTP 200
[Captures]
main_js: body regex "(/assets/[a-zA-Z0-9._-]+\\.js)"

GET {{base_url}}{{main_js}}
HTTP 200
[Asserts]
header "Content-Type" contains "javascript"

# Client-side routes fall back to index.html.
GET {{base_url}}/admin
HTTP 200
[Asserts]
header "Content-Type" contains "text/html"

GET {{base_url}}/admin/bookings
HTTP 200

GET {{base_url}}/booking/some-token
HTTP 200

GET {{base_url}}/cancel/some-token
HTTP 200

GET {{base_url}}/terms
HTTP 200

GET {{base_url}}/privacy
HTTP 200

# Vulnerability-scanner probes must 404, never serve the SPA.
GET {{base_url}}/.env
HTTP 404

GET {{base_url}}/.git/config
HTTP 404

GET {{base_url}}/wp-login.php
HTTP 404

GET {{base_url}}/phpmyadmin/
HTTP 404

GET {{base_url}}/actuator/health
HTTP 404

GET {{base_url}}/backup.sql
HTTP 404

GET {{base_url}}/index.php.bak
HTTP 404

GET {{base_url}}/debug/pprof
HTTP 404

# Unknown paths are not silently converted into the SPA.
GET {{base_url}}/definitely-not-a-route
HTTP 404

GET {{base_url}}/definitely-not-a-route.js
HTTP 404

GET {{base_url}}/api/bookings/not-a-uuid
HTTP 400
