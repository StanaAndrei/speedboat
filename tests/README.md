# Speedboat Automated Testing Framework

This directory contains automated integration and multi-node tests for Project Speedboat.

## Architecture

Speedboat relies on a distributed volunteer model where worker agents run on distinct machines with separate IP addresses and execute tasks in sandboxed runtimes.

Rather than requiring heavy, slow, and fragile Docker-in-Docker (DinD) setups or complex VM managers, tests use **Docker-out-of-Docker (DooD)**:
* Each node (`master`, `agent-1`, `agent-2`, `nats`, `registry`) runs as an independent container on a dedicated Docker bridge network (`172.28.0.0/16`).
* Each container possesses its own network namespace, hostname, and distinct IP address.
* Task containers are launched through the host Docker daemon via `/var/run/docker.sock`.
* When agents query their local network interfaces, they resolve their individual container IPs, producing distinct `WorkerID` values (e.g. `172.28.0.11-...`).

## Test Scenarios

| Test Directory | Description | Nodes | Redundancy & Consensus |
| :--- | :--- | :--- | :--- |
| `test_prime_cnt_cluster/` | Multi-node cluster running prime-counter with dynamically scalable agents | 1 Master, N Agents (configurable), NATS, Registry | Redundancy: 3, Consensus: 2 |

## Running Tests

From the repository root:

```bash
# Run all cluster tests (both 2-agent and 3-agent suites)
make test

# Run the 2-agent cluster test
make test-cluster-2-agents

# Run the 3-agent cluster test
make test-cluster-3-agents

# Run Master-only mode (waits for manual agent booting)
make test-master-only

# Clean up any lingering test containers and networks
make test-clean
```

### Manual Agent Testing Flow
To test manual volunteer onboarding step-by-step:
1. In Terminal 1, start the master:
   ```bash
   make test-master-only
   ```
2. In Terminal 2, boot up volunteer agents:
   ```bash
   # Boot 1 agent interactively (live streaming logs)
   make boot-agent

   # OR boot 2 agents in background
   make add-agents COUNT=2
   ```
3. Observe the master reach consensus, run the reducer, and exit automatically once computation finishes!

You can also run the test script directly with any desired number of agents:
```bash
# Run with 2 agents
./tests/test_prime_cnt_cluster/test.sh 2

# Run with 3 agents
./tests/test_prime_cnt_cluster/test.sh 3

# Run with 5 agents
./tests/test_prime_cnt_cluster/test.sh 5
```

## Managing & Booting Agent VMs Dynamically

You can easily boot up, inspect, and scale individual agent VMs on demand:

```bash
# Boot an agent VM interactively (foreground with live logs)
make boot-agent

# Boot an agent VM in background (detached mode, prints assigned IP)
make add-agent

# Boot multiple agent VMs in background (e.g. 3 agents)
make add-agents COUNT=3

# List all active agent VMs and their container IPs
make list-agents

# Stop all standalone agent VMs
make stop-agents
```

You can customize parameters via environment variables:
```bash
# Boot an agent pointing to a custom project or NATS endpoint
make boot-agent PROJECT_ID=my-project NATS_URL=nats://localhost:4222 RUNTIME=docker
```

## Adding New Test Scenarios

To add a new test scenario:
1. Create a new directory under `tests/` (e.g., `tests/test_wasm_cluster/`).
2. Add a `docker-compose.yaml` (using `include: [../../examples/docker-compose.yaml]`).
3. Add node entrypoints (`run-master.sh`, `run-agent.sh`).
4. Add a `test.sh` script to automate execution and assertions.
5. Add a corresponding target in the root `Makefile`.
