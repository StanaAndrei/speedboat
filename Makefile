export PATH := $(shell go env GOPATH 2>/dev/null)/bin:/opt/homebrew/bin:$(PATH)

UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
  WAILS_TAGS :=
else ifeq ($(UNAME_S),Linux)
  WAILS_TAGS := -tags webkit2_41
else
  WAILS_TAGS :=
endif

.PHONY: build build-master-cli build-agent-cli build-gui build-mac build-linux build-win clean compose-up compose-down test test-cluster test-cluster-2-agents test-cluster-3-agents test-master-only test-cluster-manual test-prime-cnt-cluster test-clean boot-agent add-agent add-agents list-agents stop-agents

build: build-master-cli build-agent-cli build-gui

build-master-cli:
	CGO_ENABLED=0 go build -o build/speedboat-master ./cmd/master

build-agent-cli:
	CGO_ENABLED=0 go build -o build/speedboat-agent ./cmd/agent

build-gui:
ifeq ($(UNAME_S),Darwin)
	@echo "Detected macOS ($(shell uname -m)) - triggering macOS GUI build..."
	@$(MAKE) build-mac
else ifeq ($(UNAME_S),Linux)
	@echo "Detected Linux - triggering Linux GUI build..."
	@$(MAKE) build-linux
else
	@case "$$(uname -s)" in \
		Darwin) $(MAKE) build-mac ;; \
		Linux)  $(MAKE) build-linux ;; \
		*) echo "Unknown operating system: $$(uname -s). Please run make build-mac, make build-linux, or make build-win directly."; exit 1 ;; \
	esac
endif

build-mac:
	@which xattr >/dev/null 2>&1 && xattr -cr gui 2>/dev/null || true
	cd gui && wails build -platform darwin/$(shell uname -m | sed 's/x86_64/amd64/') -o speedboat-gui
	@mkdir -p build
	@if [ -d gui/build/bin/speedboat-gui.app ]; then \
		rm -rf build/speedboat-gui.app; \
		cp -R gui/build/bin/speedboat-gui.app build/; \
		cp gui/build/bin/speedboat-gui.app/Contents/MacOS/speedboat-gui build/speedboat-gui; \
		cp gui/build/bin/speedboat-gui.app/Contents/MacOS/speedboat-gui gui/build/bin/speedboat-gui; \
	elif [ -f gui/build/bin/speedboat-gui ]; then \
		cp gui/build/bin/speedboat-gui build/speedboat-gui; \
	fi

build-linux:
	cd gui && wails build -tags webkit2_41 -o speedboat-gui
	@mkdir -p build
	@cp gui/build/bin/speedboat-gui build/speedboat-gui

build-win:
	cd gui && wails build -platform windows/amd64 -o speedboat_win.exe
	@mkdir -p build
	@cp gui/build/bin/speedboat_win.exe build/

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
	cd ./tests/test_prime_cnt_cluster && docker compose down -v --remove-orphans 2>/dev/null || true

clean: test-clean
	rm -rf build/* ./speedboat.db gui/build/bin/speedboat-gui.app