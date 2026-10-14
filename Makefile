.PHONY: build build-master-cli build-agent-cli build-gui clean compose-up compose-down test test-cluster test-cluster-2-agents test-cluster-3-agents test-master-only test-cluster-manual test-prime-cnt-cluster test-clean boot-agent add-agent add-agents list-agents stop-agents

build: build-master-cli build-agent-cli build-gui

build-master-cli:
	CGO_ENABLED=0 go build -o build/speedboat-master ./cmd/master

build-agent-cli:
	CGO_ENABLED=0 go build -o build/speedboat-agent ./cmd/agent

build-gui:
	cd gui && wails build -tags webkit2_41 -o speedboat-gui
	@mkdir -p build
	@cp gui/build/bin/speedboat-gui build/speedboat-gui

compose-up:
	cd ./examples && docker compose up -d

compose-down:
	cd ./examples && docker compose down

# Test targets
test: test-cluster-2-agents test-cluster-3-agents

test-cluster: test-cluster-2-agents

test-cluster-2-agents: build
	./tests/test_prime_cnt_cluster/test.sh 2

test-cluster-3-agents: build
	./tests/test_prime_cnt_cluster/test.sh 3

test-master-only: build
	./tests/test_prime_cnt_cluster/test.sh 0

test-cluster-manual: test-master-only

test-prime-cnt-cluster: test-cluster-2-agents

# Agent VM Management
boot-agent: build-agent-cli
	./scripts/boot_agent.sh

add-agent: build-agent-cli
	DETACH=true ./scripts/boot_agent.sh

add-agents: build-agent-cli
	DETACH=true COUNT=$(or $(COUNT),2) ./scripts/boot_agent.sh

list-agents:
	@count=$$(docker ps -q --filter "name=speedboat-agent" --filter "name=agent" | wc -l); \
	if [ "$$count" -eq 0 ]; then \
		echo "No agent VMs currently running."; \
	else \
		echo "Active Agent VMs ($$count):"; \
		docker ps --filter "name=speedboat-agent" --filter "name=agent" --format "{{.ID}}\t{{.Names}}\t{{.Status}}" | while read id name status; do \
			ip=$$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$$id" 2>/dev/null); \
			echo "  • [$$name] IP: $$ip ($$status)"; \
		done; \
	fi

stop-agents:
	@echo "Stopping active agent VMs..."
	@docker stop $$(docker ps -q --filter "name=speedboat-agent") 2>/dev/null || true
	@docker rm -f $$(docker ps -aq --filter "name=speedboat-agent") 2>/dev/null || true
	@echo "All standalone agent VMs stopped."

test-clean: stop-agents
	cd ./tests/test_prime_cnt_cluster && docker compose down -v --remove-orphans

clean: test-clean
	rm -rf build/* ./speedboat.db