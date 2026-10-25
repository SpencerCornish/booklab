#!/usr/bin/env bash
# Stop the functional-test app started by run-app.sh (no-op if not running).
set -euo pipefail
PID="${ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}/.functest/app.pid"

if [ -f "$PID" ]; then
  PIDVALUE="$(cat "$PID")"
  if kill -0 "$PIDVALUE" 2>/dev/null; then
    kill "$PIDVALUE" 2>/dev/null || true
    for _ in $(seq 1 20); do
      kill -0 "$PIDVALUE" 2>/dev/null || break
      sleep 0.25
    done
    kill -9 "$PIDVALUE" 2>/dev/null || true
  fi
  rm -f "$PID"
fi