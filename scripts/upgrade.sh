#!/usr/bin/env bash
# upgrade.sh — Rolling upgrade for Docker Compose production deployment.
#
# Usage: ./scripts/upgrade.sh <version> [compose-file]
#
# Example: ./scripts/upgrade.sh 1.1.0
#          ./scripts/upgrade.sh 1.1.0 docker/docker-compose.prod.yml

set -euo pipefail

VERSION="${1:-}"
COMPOSE_FILE="${2:-docker/docker-compose.prod.yml}"

if [[ -z "$VERSION" ]]; then
  echo "Usage: $0 <version> [compose-file]" >&2
  exit 1
fi

echo "==> Upgrading LLM Gateway to v${VERSION}"
echo "    Compose file: ${COMPOSE_FILE}"

# 1. Pull new images.
echo "==> Pulling images..."
GATEWAY_VERSION="${VERSION}" docker compose -f "${COMPOSE_FILE}" pull gateway admin

# 2. Run database migrations.
echo "==> Running database migrations..."
GATEWAY_VERSION="${VERSION}" docker compose -f "${COMPOSE_FILE}" run --rm \
  -e RUN_MIGRATIONS=1 gateway ./migrate up || {
  echo "ERROR: Migration failed. Aborting upgrade." >&2
  exit 1
}

# 3. Rolling restart of the gateway service (zero-downtime if replicas > 1).
echo "==> Restarting gateway..."
GATEWAY_VERSION="${VERSION}" docker compose -f "${COMPOSE_FILE}" up -d --no-deps gateway

# 4. Wait for health check.
echo "==> Waiting for gateway to become healthy..."
RETRIES=30
until curl -sf "http://localhost:8080/health/ready" > /dev/null 2>&1; do
  RETRIES=$((RETRIES - 1))
  if [[ $RETRIES -le 0 ]]; then
    echo "ERROR: Gateway did not become healthy after upgrade." >&2
    echo "       Run: docker compose logs -f gateway" >&2
    exit 1
  fi
  sleep 2
done

echo "==> Gateway is healthy."

# 5. Upgrade admin UI.
echo "==> Restarting admin UI..."
GATEWAY_VERSION="${VERSION}" docker compose -f "${COMPOSE_FILE}" up -d --no-deps admin

echo "==> Upgrade to v${VERSION} complete."
