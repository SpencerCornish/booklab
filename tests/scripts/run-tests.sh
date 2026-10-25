#!/usr/bin/env bash
# Orchestrates one functional suite type end-to-end:
#   usage: run-tests.sh hurl|e2e
#   1. start the compose stack (Postgres + Mailpit)
#   2. reset the DB, start the app binary, run the suite(s), resetting state
#      between Hurl files so suite order never matters
#   3. tear everything down (even on failure)
set -uo pipefail
# shellcheck source=tests/scripts/lib.sh
source "${ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}/tests/scripts/lib.sh"

MODE="${1:-}"
if [ "$MODE" != "hurl" ] && [ "$MODE" != "e2e" ]; then
  echo "usage: run-tests.sh hurl|e2e" >&2
  exit 2
fi

HURL_BIN="${HURL:-hurl}"
STATUS=0

cleanup() {
  "$ROOT/tests/scripts/stop-app.sh" || true
  "$ROOT/tests/scripts/stack-down.sh" || true
}
trap cleanup EXIT

mkdir -p "$ROOT/.functest"

"$ROOT/tests/scripts/stack-up.sh"
"$ROOT/tests/scripts/run-app.sh"
# reset AFTER first app boot: migrations + bootstrap admin run on startup.
"$ROOT/tests/scripts/reset-db.sh"

case "$MODE" in
  hurl)
    # Near-term dates for the availability day-strip endpoint (it is anchored
    # to "now" — it feeds the picker's 60-day window). Computed in the app's
    # settings timezone so the calendar date is unambiguous around midnight.
    TOMORROW="$(TZ=America/Denver date -d tomorrow +%F)"
    DAY_AFTER="$(TZ=America/Denver date -d 'tomorrow + 1 day' +%F)"
    FIRST=1
    for f in "$ROOT"/tests/hurl/suites/*.hurl; do
      [ "$FIRST" -eq 0 ] && "$ROOT/tests/scripts/reset-db.sh"
      FIRST=0
      name="$(basename "$f" .hurl)"
      echo "── hurl suite: $name"
      "$HURL_BIN" --test \
        --variables-file "$ROOT/tests/hurl/vars/functional.env" \
        --variable "run_id=$(date +%s)" \
        --variable "tomorrow=$TOMORROW" \
        --variable "day_after=$DAY_AFTER" \
        --report-junit "$ROOT/.functest/junit-${name}.xml" \
        "$f" || STATUS=$?
    done
    if [ "$STATUS" -eq 0 ]; then echo "hurl suites passed"; else echo "hurl suites FAILED ($STATUS)" >&2; fi
    ;;
  e2e)
    if [ -n "${STRIPE_TEST_PUBLISHABLE_KEY:-}" ]; then
      echo "e2e: rebuilding app with real Stripe publishable key (payment spec mode)"
      (cd "$ROOT" && VITE_STRIPE_PUBLISHABLE_KEY="$STRIPE_TEST_PUBLISHABLE_KEY" make build) || STATUS=$?
    fi
    if [ -n "${STRIPE_TEST_SECRET_KEY:-}" ]; then
      export STRIPE_SECRET_KEY="$STRIPE_TEST_SECRET_KEY"
      export STRIPE_FAKE=0
    fi
    (cd "$ROOT/tests/e2e"
      pnpm install --frozen-lockfile
      pnpm exec playwright install chromium
      pnpm exec playwright test "$@"
    ) || STATUS=$?
    if [ "$STATUS" -eq 0 ]; then echo "e2e suites passed"; else echo "e2e suites FAILED ($STATUS)" >&2; fi
    ;;
esac

exit "$STATUS"