#!/usr/bin/env bash
# Orchestrates one functional suite type end-to-end:
#   usage: run-tests.sh hurl|e2e
#   1. start the compose stack (Postgres + Mailpit)
#   2. reset the DB, start the app binary, run the suite(s), resetting state
#      between Hurl files so suite order never matters
#   3. tear everything down (even on failure)
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=tests/scripts/lib.sh
source "$ROOT/tests/scripts/lib.sh"

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
    FIRST=1
    for f in "$ROOT"/tests/hurl/suites/*.hurl; do
      [ "$FIRST" -eq 0 ] && "$ROOT/tests/scripts/reset-db.sh"
      FIRST=0
      name="$(basename "$f" .hurl)"
      echo "── hurl suite: $name"
      "$HURL_BIN" --test \
        --variables-file "$ROOT/tests/hurl/vars/functional.env" \
        --variable "run_id=$(date +%s)" \
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