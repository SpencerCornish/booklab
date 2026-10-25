# Health + public content endpoints. No authentication, no state.
# Runs against the real binary with default-seeded settings.

# Liveness probe: fast, dependency-free, no auth.
GET {{base_url}}/api/health
HTTP 200
[Asserts]
jsonpath "$.status" == "ok"
header "Content-Type" contains "application/json"

# Public settings reflect the default settings row (001_initial.up.sql seed).
GET {{base_url}}/api/settings/public
HTTP 200
[Asserts]
jsonpath "$.resource_name" == "The Space"
jsonpath "$.hourly_rate_cents" == 1500
jsonpath "$.currency" == "usd"
jsonpath "$.timezone" == "America/Denver"
jsonpath "$.bookable_start" == "09:00"
jsonpath "$.bookable_end" == "21:00"
jsonpath "$.min_hours" == 1
jsonpath "$.max_hours" == 8
jsonpath "$.min_lead_hours" == 0
jsonpath "$.referral_sources" exists

GET {{base_url}}/api/terms
HTTP 200
[Asserts]
jsonpath "$.content" exists

GET {{base_url}}/api/privacy
HTTP 200
[Asserts]
jsonpath "$.content" exists

# Unknown API path must not fall through to the SPA catch-all.
GET {{base_url}}/api/nope
HTTP 404
