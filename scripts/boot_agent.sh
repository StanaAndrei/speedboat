#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Configuration options (can be overridden via environment variables)
COUNT="${COUNT:-1}"
PROJECT_ID="${PROJECT_ID:-prime-counter}"
RUNTIME="${RUNTIME:-docker}"
ONCE="${ONCE:-false}"
DETACH="${DETACH:-false}"
NETWORK="${NETWORK:-}"
NATS_URL="${NATS_URL:-}"

# Ensure static agent binary exists
if [ ! -f "${REPO_ROOT}/build/speedboat-agent" ]; then
  echo "🔨 Building static agent binary..."
  make -C "${REPO_ROOT}" build-agent-cli
fi

# Ensure test node image exists
if ! docker image inspect speedboat-test-node:latest >/dev/null 2>&1; then
  echo "🔨 Building test node container image..."
  docker build -t speedboat-test-node:latest "${REPO_ROOT}/tests/test_prime_cnt_cluster"
fi

# Auto-detect running network and NATS endpoint if not explicitly provided
if [ -z "${NETWORK}" ] || [ -z "${NATS_URL}" ]; then
  if docker ps --filter "name=speedboat-nats" --format '{{.Names}}' | grep -q "speedboat-nats"; then
    DETECTED_NET=$(docker inspect -f '{{range $net, $v := .NetworkSettings.Networks}}{{$net}}{{end}}' speedboat-nats 2>/dev/null | awk '{print $1}')
    NETWORK="${NETWORK:-${DETECTED_NET}}"
    NATS_URL="${NATS_URL:-nats://speedboat-nats:4222}"
  elif docker network inspect test_prime_cnt_cluster_default >/dev/null 2>&1; then
    NETWORK="${NETWORK:-test_prime_cnt_cluster_default}"
    NATS_URL="${NATS_URL:-nats://nats:4222}"
  elif docker network inspect examples_default >/dev/null 2>&1; then
    NETWORK="${NETWORK:-examples_default}"
    NATS_URL="${NATS_URL:-nats://speedboat-nats:4222}"
  else
    # Fallback to standard bridge
    NETWORK="${NETWORK:-bridge}"
    NATS_URL="${NATS_URL:-nats://172.17.0.1:4222}"
  fi
fi

# Single agent in foreground mode
if [ "${COUNT}" -eq 1 ] && [ "${DETACH}" != "true" ]; then
  RAND_SUFFIX=$(head -c 3 /dev/urandom 2>/dev/null | xxd -p 2>/dev/null || date +%s%N | tail -c 6)
  CONTAINER_NAME="${NAME_PREFIX:-speedboat-agent}-${RAND_SUFFIX}"

  echo "============================================================"
  echo "🚀 Booting Agent VM (Interactive Foreground)"
  echo "   Container Name: ${CONTAINER_NAME}"
  echo "   Docker Network: ${NETWORK}"
  echo "   NATS Endpoint:  ${NATS_URL}"
  echo "   Project ID:     ${PROJECT_ID}"
  echo "   Runtime Engine: ${RUNTIME}"
  echo "   Once Mode:      ${ONCE}"
  echo "   (Press Ctrl+C to stop agent)"
  echo "============================================================"

  set +e
  docker run --rm -it \
    --name "${CONTAINER_NAME}" \
    --network "${NETWORK}" \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v "${REPO_ROOT}:${REPO_ROOT}" \
    -e REPO_ROOT="${REPO_ROOT}" \
    -e NATS_URL="${NATS_URL}" \
    -e PROJECT_ID="${PROJECT_ID}" \
    -e RUNTIME="${RUNTIME}" \
    -e ONCE="${ONCE}" \
    speedboat-test-node:latest \
    /bin/bash "${REPO_ROOT}/tests/test_prime_cnt_cluster/run-agent.sh" "${CONTAINER_NAME}"
  EXIT_CODE=$?
  set -e

  # Clean exit on: 0 (normal), 130 (SIGINT / Ctrl+C), 137 (SIGKILL from docker stop), 143 (SIGTERM)
  if [ ${EXIT_CODE} -eq 0 ] || [ ${EXIT_CODE} -eq 130 ] || [ ${EXIT_CODE} -eq 137 ] || [ ${EXIT_CODE} -eq 143 ]; then
    echo -e "\n🛑 Agent VM '${CONTAINER_NAME}' stopped cleanly."
    exit 0
  fi
  exit ${EXIT_CODE}
fi

# Multi-agent or detached mode
echo "============================================================"
echo "🚀 Booting ${COUNT} Agent VM(s) (Detached Background)"
echo "   Docker Network: ${NETWORK}"
echo "   NATS Endpoint:  ${NATS_URL}"
echo "   Project ID:     ${PROJECT_ID}"
echo "   Runtime Engine: ${RUNTIME}"
echo "============================================================"

for ((i=1; i<=COUNT; i++)); do
  RAND_SUFFIX=$(head -c 3 /dev/urandom 2>/dev/null | xxd -p 2>/dev/null || date +%s%N | tail -c 6)
  CONTAINER_NAME="${NAME_PREFIX:-speedboat-agent}-${RAND_SUFFIX}"

  CID=$(docker run -d \
    --name "${CONTAINER_NAME}" \
    --network "${NETWORK}" \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v "${REPO_ROOT}:${REPO_ROOT}" \
    -e REPO_ROOT="${REPO_ROOT}" \
    -e NATS_URL="${NATS_URL}" \
    -e PROJECT_ID="${PROJECT_ID}" \
    -e RUNTIME="${RUNTIME}" \
    -e ONCE="${ONCE}" \
    speedboat-test-node:latest \
    /bin/bash "${REPO_ROOT}/tests/test_prime_cnt_cluster/run-agent.sh" "${CONTAINER_NAME}")

  # Brief sleep to allow network attachment
  sleep 0.5
  AGENT_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "${CONTAINER_NAME}" 2>/dev/null || echo "allocating...")

  echo "[$i/${COUNT}] ✅ Booted ${CONTAINER_NAME}"
  echo "       Container ID: ${CID:0:12}"
  echo "       IP Address:   ${AGENT_IP}"
  echo "       Logs Command: docker logs -f ${CONTAINER_NAME}"
done

echo "------------------------------------------------------------"
echo "💡 Use 'make list-agents' to view all active agent VMs."
echo "💡 Use 'make stop-agents' to terminate all spawned agents."
echo "============================================================"
