#!/usr/bin/env bash
# Start the functional-test containers (Postgres + Mailpit).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=tests/scripts/lib.sh
source "$ROOT/tests/scripts/lib.sh"

"${COMPOSE[@]}" up -d --wait
echo "functest stack up (app port ${FUNC_APP_PORT}, Mailpit UI $(mailpit_base_url))"