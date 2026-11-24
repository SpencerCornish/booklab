# E-ink door-sign displays (TRMNL firmware protocol). The device side needs no
# browser session; the admin side approves the device. See docs/eink-display.md.

# ── Device setup ─────────────────────────────────────────────────────────────
# Invalid MAC: 404, which the firmware treats as "not registered".
GET {{base_url}}/api/setup
ID: not-a-mac
HTTP 404
[Asserts]
jsonpath "$.status" == 404

GET {{base_url}}/api/setup
ID: aa:bb:cc:dd:ee:42
FW-Version: 1.6.0
Model: og
HTTP 200
[Captures]
api_key: jsonpath "$.api_key"
friendly_id: jsonpath "$.friendly_id"
[Asserts]
jsonpath "$.status" == 200
jsonpath "$.api_key" isString
jsonpath "$.friendly_id" matches "^[A-Z2-9]{6}$"

# ── Pending: waiting-for-approval screen, polled every minute ────────────────
GET {{base_url}}/api/display
ID: AA:BB:CC:DD:EE:42
Access-Token: {{api_key}}
Battery-Voltage: 4.05
RSSI: -58
FW-Version: 1.6.0
Width: 800
Height: 480
HTTP 200
[Captures]
pending_image: jsonpath "$.image_url"
pending_filename: jsonpath "$.filename"
[Asserts]
jsonpath "$.status" == 0
jsonpath "$.refresh_rate" == 60
jsonpath "$.image_url" startsWith "{{base_url}}/api/display/image/"
jsonpath "$.filename" startsWith "booklab-"

# The firmware picks PNG decoding from the Content-Type.
GET {{pending_image}}
HTTP 200
[Asserts]
header "Content-Type" == "image/png"
bytes startsWith hex,89504e470d0a1a0a;

# Unknown key from a known MAC: not adopted (status 202 = unclaimed).
GET {{base_url}}/api/display
ID: AA:BB:CC:DD:EE:42
Access-Token: not-the-right-key-0123456789abcdef
HTTP 200
[Asserts]
jsonpath "$.status" == 202

# ── Admin approves ───────────────────────────────────────────────────────────
POST {{base_url}}/api/admin/login
{"username": "{{admin_user}}", "password": "{{admin_pass}}"}
HTTP 200

GET {{base_url}}/api/admin/displays
HTTP 200
[Captures]
csrf: header "Set-Cookie" regex "booklab_csrf=([^;]+)"
display_id: jsonpath "$[0].id"
[Asserts]
jsonpath "$" count == 1
jsonpath "$[0].status" == "pending"
jsonpath "$[0].friendly_id" == "{{friendly_id}}"
jsonpath "$[0].mac_address" == "AA:BB:CC:DD:EE:42"
jsonpath "$[0].battery_low" == false
jsonpath "$[0].api_key_hash" not exists

GET {{base_url}}/api/admin/displays/preview.png
HTTP 200
[Asserts]
header "Content-Type" == "image/png"

PATCH {{base_url}}/api/admin/displays/{{display_id}}
X-CSRF-Token: {{csrf}}
{"status": "approved", "name": "Darkroom door"}
HTTP 200
[Asserts]
jsonpath "$.status" == "approved"
jsonpath "$.name" == "Darkroom door"
jsonpath "$.approved_at" exists

PATCH {{base_url}}/api/admin/displays/{{display_id}}
X-CSRF-Token: {{csrf}}
{"status": "bogus"}
HTTP 400

# ── Approved: the schedule ───────────────────────────────────────────────────
GET {{base_url}}/api/display
ID: AA:BB:CC:DD:EE:42
Access-Token: {{api_key}}
HTTP 200
[Captures]
sign_image: jsonpath "$.image_url"
[Asserts]
jsonpath "$.status" == 0
jsonpath "$.filename" != "{{pending_filename}}"
jsonpath "$.maximum_compatibility" == true
jsonpath "$.refresh_rate" >= 60
jsonpath "$.refresh_rate" <= 3600

GET {{sign_image}}
HTTP 200
[Asserts]
header "Content-Type" == "image/png"

# ── Revoke: old image URLs stop working; setup is refused ────────────────────
PATCH {{base_url}}/api/admin/displays/{{display_id}}
X-CSRF-Token: {{csrf}}
{"status": "revoked"}
HTTP 200

GET {{sign_image}}
HTTP 404

GET {{base_url}}/api/setup
ID: AA:BB:CC:DD:EE:42
HTTP 404

DELETE {{base_url}}/api/admin/displays/{{display_id}}
X-CSRF-Token: {{csrf}}
HTTP 204
