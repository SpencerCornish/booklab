#!/usr/bin/env bash
# Reset the functional-test database to a clean state.
#
# Keeps admin_users (the bootstrap admin is only created at server boot) and resets
# the settings row to defaults so suites that mutate settings stay independent.
# Clears state tables between suites so ordering never matters.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=tests/scripts/lib.sh
source "$ROOT/tests/scripts/lib.sh"

if ! db_ready; then
  echo "reset-db: test Postgres container is not running (run tests/scripts/stack-up.sh first)" >&2
  exit 1
fi

"${COMPOSE[@]}" exec -T db psql -U booklab -d booklab -v ON_ERROR_STOP=1 <<'SQL'
TRUNCATE bookings, closures, admin_sessions, login_attempts;
TRUNCATE settings;
INSERT INTO settings DEFAULT VALUES;
SQL

# Best-effort Mailpit clear so suites that assert on email content never see
# messages from earlier suites. Older Mailpit versions may not support this.
curl -sf -X DELETE "$(mailpit_base_url)/api/v1/messages" >/dev/null 2>&1 || true

echo "functest db reset"