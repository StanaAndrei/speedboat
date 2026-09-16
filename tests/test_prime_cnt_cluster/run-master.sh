#!/usr/bin/env bash
set -euo pipefail

echo "=== [Master Node] Initializing ==="
MY_IP=$(hostname -i | awk '{print $1}')
echo "=== [Master Node] Hostname: $(hostname), IP: ${MY_IP} ==="

NATS_ENDPOINT="${NATS_URL:-nats://nats:4222}"
NATS_HOST=$(echo "${NATS_ENDPOINT}" | sed -E 's|^nats://([^:]+):.*|\1|')
NATS_PORT=$(echo "${NATS_ENDPOINT}" | sed -E 's|^nats://[^:]+:([0-9]+).*|\1|')

echo "=== [Master Node] Waiting for NATS at ${NATS_HOST}:${NATS_PORT}... ==="
until nc -z -w 2 "${NATS_HOST}" "${NATS_PORT}"; do
  sleep 1
done
echo "=== [Master Node] NATS is ready! ==="

echo "=== [Master Node] Waiting for Docker Registry at registry:5000... ==="
until nc -z -w 2 "registry" 5000; do
  sleep 1
done
echo "=== [Master Node] Docker Registry is ready! ==="

PROJECT_DIR="${REPO_ROOT:-/app}/examples/prime-cnt"
MASTER_BIN="${REPO_ROOT:-/app}/build/speedboat-master"

echo "=== [Master Node] Running speedboat-master ==="
exec "${MASTER_BIN}" run \
  -p "${PROJECT_DIR}" \
  --nats-url "${NATS_ENDPOINT}" \
  --registry "${DOCKER_REGISTRY:-localhost:5000}"
