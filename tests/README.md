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
| `test_prime_cnt_cluster/` | Multi-node cluster running the C++ prime-counter task | 1 Master, 2 Agents, NATS, Registry | Redundancy: 3, Consensus: 2 |

## Running Tests

From the repository root:

```bash
# Run all cluster tests
make test

# Run the prime-counter multi-node cluster test specifically
make test-prime-cnt-cluster

# Clean up any lingering test containers and networks
make test-clean
```

You can also run the test directly:
```bash
./tests/test_prime_cnt_cluster/test.sh
```

## Adding New Test Scenarios

To add a new test scenario:
1. Create a new directory under `tests/` (e.g., `tests/test_wasm_cluster/`).
2. Add a `docker-compose.yaml` (using `include: [../../examples/docker-compose.yaml]`).
3. Add node entrypoints (`run-master.sh`, `run-agent.sh`).
4. Add a `test.sh` script to automate execution and assertions.
5. Add a corresponding target in the root `Makefile`.
