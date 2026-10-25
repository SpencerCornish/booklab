#!/usr/bin/env bash
# Stop the functional-test containers. The test stack is disposable (no data
# volumes): the next run starts from a fresh database and Mailpit.
set -euo pipefail
# shellcheck source=tests/scripts/lib.sh
source "${ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}/tests/scripts/lib.sh"

"$ROOT/tests/scripts/stop-app.sh" || true
"${COMPOSE[@]}" down