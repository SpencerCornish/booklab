#!/usr/bin/env bash
# Docker smoke test: build and run the PRODUCTION compose stack (the real
# deployment unit — SPA baked into the image, migrations on a fresh volume),
# then probe it with a slim Hurl pass.
#
# Never touches any real .env: it swaps in a disposable one and restores the
# original afterwards. Data lives in a temp dir, not ./data/postgres.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=tests/scripts/lib.sh
source "$ROOT/tests/scripts/lib.sh"

COMPOSE_CMD="${FUNCTEST_COMPOSE_CMD:-docker compose}"
# The override only disables SELinux volume labeling (rootless podman); it
# changes nothing under Docker, so the prod stack itself is what's tested.
COMPOSE=($COMPOSE_CMD -f "$ROOT/docker-compose.prod.yml" -f "$ROOT/tests/smoke/compose.override.yml" -p "booklab-smoke")

SMOKE_PORT="${SMOKE_PORT:-8091}"
SMOKE_ADMIN_PASS="smoke-admin-pass"
PGDATA="$(mktemp -d "${TMPDIR:-/tmp}/booklab-smoke-pgdata.XXXXXX")"

ENV_FILE="$ROOT/.env"
SAVED_ENV=""
if [ -f "$ENV_FILE" ]; then
  SAVED_ENV="$(mktemp "${TMPDIR:-/tmp}/booklab-smoke-env.XXXXXX")"
  cp "$ENV_FILE" "$SAVED_ENV"
fi

restore_env() {
  if [ -n "$SAVED_ENV" ]; then
    mv "$SAVED_ENV" "$ENV_FILE"
  else
    rm -f "$ENV_FILE"
  fi
  "${COMPOSE[@]}" down -v >/dev/null 2>&1 || true
  # The volume files may carry container SELinux labels the host can't unlink
  # under rootless podman — leftovers in $TMPDIR are harmless.
  rm -rf "$PGDATA" 2>/dev/null || true
}
trap restore_env EXIT

STATUS=0

# Disposable .env for the production compose file (env_file: .env).
cat > "$ENV_FILE" <<EOF
DATABASE_URL=ignored-by-compose
POSTGRES_PASSWORD=booklab-smoke
POSTGRES_DATA_PATH=$PGDATA
APP_PUBLISH_PORT=$SMOKE_PORT
PORT=8080
STRIPE_SECRET_KEY=sk_test_functional_placeholder
STRIPE_FAKE=1
SMTP_HOST=localhost
SMTP_PORT=1025
SMTP_USER=
SMTP_PASS=
SMTP_FROM=BookLab <noreply@localhost>
APP_URL=http://localhost:$SMOKE_PORT
CORS_ALLOWED_ORIGINS=http://localhost:$SMOKE_PORT
ADMIN_USER=$FUNC_ADMIN_USER
ADMIN_PASS=$SMOKE_ADMIN_PASS
VITE_STRIPE_PUBLISHABLE_KEY=pk_test_smoke_placeholder
EOF

echo "── building and starting production stack on :$SMOKE_PORT"
"${COMPOSE[@]}" up -d --build

for _ in $(seq 1 120); do
  if curl -sf "http://localhost:$SMOKE_PORT/api/health" >/dev/null 2>&1; then break; fi
  sleep 1
done
if ! curl -sf "http://localhost:$SMOKE_PORT/api/health" >/dev/null 2>&1; then
  echo "smoke: stack never became healthy" >&2
  "${COMPOSE[@]}" logs --tail 40 || true
  exit 1
fi
echo "smoke: stack healthy"

HURL_BIN="${HURL:-hurl}"
"$HURL_BIN" --test \
  --variable "base_url=http://localhost:$SMOKE_PORT" \
  --variable "admin_user=$FUNC_ADMIN_USER" \
  --variable "admin_pass=$SMOKE_ADMIN_PASS" \
  --variable "run_id=$(date +%s)" \
  "$ROOT/tests/hurl/suites/00_health.hurl" \
  "$ROOT/tests/hurl/suites/06_spa_routing.hurl" \
  "$ROOT/tests/smoke/admin_check.hurl" || STATUS=$?

if [ "$STATUS" -eq 0 ]; then
  echo "docker smoke passed"
else
  echo "docker smoke FAILED" >&2
fi
exit "$STATUS"