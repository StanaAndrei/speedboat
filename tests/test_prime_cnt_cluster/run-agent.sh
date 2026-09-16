#!/usr/bin/env bash
set -euo pipefail

WORKER_LABEL="${1:-$(hostname)}"
echo "=== [${WORKER_LABEL}] Initializing ==="
MY_IP=$(hostname -i | awk '{print $1}')
echo "=== [${WORKER_LABEL}] Hostname: $(hostname), IP: ${MY_IP} ==="

NATS_ENDPOINT="${NATS_URL:-nats://nats:4222}"
NATS_HOST=$(echo "${NATS_ENDPOINT}" | sed -E 's|^nats://([^:]+):.*|\1|')
NATS_PORT=$(echo "${NATS_ENDPOINT}" | sed -E 's|^nats://[^:]+:([0-9]+).*|\1|')

echo "=== [${WORKER_LABEL}] Waiting for NATS at ${NATS_HOST}:${NATS_PORT}... ==="
until nc -z -w 2 "${NATS_HOST}" "${NATS_PORT}"; do
  sleep 1
done
echo "=== [${WORKER_LABEL}] NATS is ready! Starting agent worker... ==="

AGENT_BIN="${REPO_ROOT:-/app}/build/speedboat-agent"

# Run agent with once flag, retrying on transient connection/stream errors
MAX_RETRIES=30
for ((i=1; i<=MAX_RETRIES; i++)); do
  if "${AGENT_BIN}" run -p prime-counter --nats-url "${NATS_ENDPOINT}" --runtime docker --once; then
    echo "=== [${WORKER_LABEL}] Agent completed successfully! ==="
    exit 0
  fi
  echo "=== [${WORKER_LABEL}] Agent exited with non-zero status. Retrying ($i/$MAX_RETRIES)... ==="
  sleep 1
done

echo "=== [${WORKER_LABEL}] Failed after $MAX_RETRIES retries. ==="
exit 1
