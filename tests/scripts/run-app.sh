#!/usr/bin/env bash
# Start the BookLab binary in functional-test mode and wait for /api/health.
# The binary must already be built (make build-api / make build).
#
# All sensitive values are placeholders: STRIPE_FAKE=1 makes every Stripe call
# hermetic, and SMTP points at the Mailpit container from the test stack.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=tests/scripts/lib.sh
source "$ROOT/tests/scripts/lib.sh"

BIN="${FUNCTEST_APP_BIN:-$ROOT/booklab}"

# Pin the server's local timezone. pgx decodes timestamptz into time.Local, so
# JSON serialization of booking times would otherwise depend on the host's TZ
# (dev laptop vs UTC-based CI runners vs production containers). UTC matches
# production (the Docker image ships no TZ setting).
export TZ=UTC
if [ ! -x "$BIN" ]; then
  echo "run-app: $BIN not found or not executable; run 'make build' first" >&2
  exit 1
fi

mkdir -p "$ROOT/.functest"
LOG="$ROOT/.functest/app.log"
PID="$ROOT/.functest/app.pid"

export DATABASE_URL="${FUNC_DATABASE_URL:-postgres://booklab:booklab@localhost:${FUNCTEST_POSTGRES_PORT}/booklab?sslmode=disable}"
export PORT="$FUNC_APP_PORT"
export STRIPE_SECRET_KEY="${STRIPE_SECRET_KEY:-sk_test_functional_placeholder}"
export STRIPE_FAKE="${STRIPE_FAKE:-1}"
export SMTP_HOST="${SMTP_HOST:-localhost}"
export SMTP_PORT="$FUNCTEST_SMTP_PORT"
export SMTP_USER="${SMTP_USER:-}"
export SMTP_PASS="${SMTP_PASS:-}"
export SMTP_FROM="${SMTP_FROM:-BookLab <noreply@localhost>}"
export APP_URL="$(app_base_url)"
export CORS_ALLOWED_ORIGINS="$APP_URL"
export ADMIN_USER="$FUNC_ADMIN_USER"
export ADMIN_PASS="$FUNC_ADMIN_PASS"

# Refuse to run if a previous instance is still alive.
"$ROOT/tests/scripts/stop-app.sh" >/dev/null 2>&1 || true

nohup "$BIN" > "$LOG" 2>&1 &
echo $! > "$PID"

for _ in $(seq 1 120); do
  if curl -sf "$(app_base_url)/api/health" >/dev/null 2>&1; then
    echo "functest app ready on $(app_base_url) (pid $(cat "$PID"))"
    exit 0
  fi
  if ! kill -0 "$(cat "$PID")" 2>/dev/null; then
    echo "run-app: app exited during startup — last log lines:" >&2
    tail -n 30 "$LOG" >&2
    exit 1
  fi
  sleep 0.5
done

echo "run-app: timed out waiting for $(app_base_url)/api/health — last log lines:" >&2
tail -n 30 "$LOG" >&2
exit 1