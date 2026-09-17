#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

echo "============================================================"
echo "🚀 Project Speedboat: Automated Multi-Node Cluster Test"
echo "📁 Repository Root: ${REPO_ROOT}"
echo "============================================================"

NUM_AGENTS="${1:-2}"

# 1. Build Go static binaries
echo -e "\n[1/4] 🔨 Compiling static binaries (speedboat-master, speedboat-agent)..."
make -C "${REPO_ROOT}" build

# 2. Cleanup any conflicting containers
echo -e "\n[2/4] 🧹 Cleaning up any previous test or example containers..."
docker stop speedboat-nats speedboat-registry speedboat-test-master $(docker ps -q --filter "name=agent") 2>/dev/null || true
docker rm -f speedboat-nats speedboat-registry speedboat-test-master $(docker ps -aq --filter "name=agent") 2>/dev/null || true
REPO_ROOT="${REPO_ROOT}" docker compose -f "${SCRIPT_DIR}/docker-compose.yaml" down -v --remove-orphans 2>/dev/null || true

# 3. Execute the multi-node test
echo -e "\n[3/4] 🚢 Launching cluster test nodes:"
if [ "${NUM_AGENTS}" -eq 0 ]; then
  echo "       - Master Node   (IP: 172.28.0.10)"
  echo "       - Agent Nodes   (0 auto-started; manual mode)"
  echo "       - NATS Server   (IP: 172.28.0.2 / nats:4222)"
  echo "       - Registry      (IP: 172.28.0.3 / registry:5000)"
  echo "------------------------------------------------------------"
  echo "💡 Master will start and wait for volunteer agents to join."
  echo "💡 Run either of the following in another terminal to join:"
  echo "      make boot-agent          # boot 1 agent in foreground"
  echo "   OR make add-agents COUNT=2  # boot 2 agents in background"
  echo "------------------------------------------------------------"
else
  echo "       - Master Node   (IP: 172.28.0.10)"
  echo "       - Agent Nodes   (${NUM_AGENTS} workers dynamically allocated on 172.28.0.0/16)"
  echo "       - NATS Server   (IP: 172.28.0.2 / nats:4222)"
  echo "       - Registry      (IP: 172.28.0.3 / registry:5000)"
  echo "------------------------------------------------------------"
fi

if ! docker image inspect speedboat-test-node:latest >/dev/null 2>&1; then
  echo "🔨 Building test node container image..."
  docker build -t speedboat-test-node:latest "${SCRIPT_DIR}"
fi

export REPO_ROOT="${REPO_ROOT}"

# Launch docker compose with abort-on-container-exit, tied to master's exit code
set +e
docker compose -f "${SCRIPT_DIR}/docker-compose.yaml" up \
  --scale agent="${NUM_AGENTS}" \
  --abort-on-container-exit \
  --exit-code-from master
MASTER_EXIT_CODE=$?
set -e

# 4. Teardown and Evaluation
echo "------------------------------------------------------------"
echo -e "\n[4/4] 📊 Evaluating test results..."

if [ ${MASTER_EXIT_CODE} -eq 0 ]; then
  echo "✅ TEST PASSED: Master completed successfully with consensus verification!"
else
  echo "❌ TEST FAILED: Master exited with code ${MASTER_EXIT_CODE}."
fi

echo -e "\n🧹 Cleaning up test cluster..."
docker stop $(docker ps -q --filter "name=speedboat-agent") 2>/dev/null || true
docker rm -f $(docker ps -aq --filter "name=speedboat-agent") 2>/dev/null || true
REPO_ROOT="${REPO_ROOT}" docker compose -f "${SCRIPT_DIR}/docker-compose.yaml" down -v --remove-orphans

if [ ${MASTER_EXIT_CODE} -ne 0 ]; then
  exit ${MASTER_EXIT_CODE}
fi

echo "🎉 All tests completed successfully!"
