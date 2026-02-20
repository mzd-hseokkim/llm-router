#!/usr/bin/env bash
# start.sh - Build and start the gateway server
# Loads .env.local from the project root if it exists.

set -e

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ENV_FILE="$ROOT/.env.local"

if [ -f "$ENV_FILE" ]; then
    while IFS='=' read -r key value; do
        [[ -z "$key" || "$key" == \#* ]] && continue
        export "$key=$value"
        echo "  loaded: $key"
    done < "$ENV_FILE"
    echo ""
fi

cd "$ROOT"
echo "Starting gateway..."
exec go run ./cmd/gateway
