#!/usr/bin/env bash
# restart.sh - Stop the running gateway and start it again

PORT="${1:-8080}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "Restarting gateway on port $PORT..."

"$SCRIPT_DIR/stop.sh" "$PORT"

sleep 0.5

exec "$SCRIPT_DIR/start.sh"
