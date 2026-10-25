#!/usr/bin/env bash
# Shared functional-test configuration. Sourced by every script in this directory.
# All values can be overridden via environment variables (used by CI to map ports).
ROOT="${ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
FUNCTEST_PROJECT="${FUNCTEST_PROJECT:-booklab-functest}"
FUNCTEST_POSTGRES_PORT="${FUNCTEST_POSTGRES_PORT:-5433}"
FUNCTEST_SMTP_PORT="${FUNCTEST_SMTP_PORT:-1026}"
FUNCTEST_MAILPIT_UI_PORT="${FUNCTEST_MAILPIT_UI_PORT:-8027}"
FUNC_APP_PORT="${FUNC_APP_PORT:-8090}"
FUNC_ADMIN_USER="${FUNC_ADMIN_USER:-admin}"
FUNC_ADMIN_PASS="${FUNC_ADMIN_PASS:-functest-admin-pass}"

COMPOSE_CMD="${FUNCTEST_COMPOSE_CMD:-docker compose}"
# shellcheck disable=SC2206  # word splitting is intentional here
COMPOSE=($COMPOSE_CMD -f "$ROOT/tests/compose/docker-compose.test.yml" -p "$FUNCTEST_PROJECT")

db_ready() {
  "${COMPOSE[@]}" exec -T db pg_isready -U booklab >/dev/null 2>&1
}

app_base_url() {
  echo "http://localhost:${FUNC_APP_PORT}"
}

mailpit_base_url() {
  echo "http://localhost:${FUNCTEST_MAILPIT_UI_PORT}"
}

# wait_healthy <url> [timeout_seconds] — poll until a URL answers, non-zero on timeout.
wait_healthy() {
  local url="$1" timeout="${2:-60}"
  local deadline=$(( $(date +%s) + timeout ))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    curl -sf "$url" >/dev/null 2>&1 && return 0
    sleep 0.5
  done
  echo "timeout waiting for $url" >&2
  return 1
}