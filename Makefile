.PHONY: build build-master-cli build-agent-cli clean compose-up compose-down test test-cluster test-cluster-2-agents test-cluster-3-agents test-prime-cnt-cluster test-clean

build: build-master-cli build-agent-cli


build-master-cli:
	CGO_ENABLED=0 go build -o build/speedboat-master ./cmd/master

build-agent-cli:
	CGO_ENABLED=0 go build -o build/speedboat-agent ./cmd/agent

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

test-prime-cnt-cluster: test-cluster-2-agents

test-clean:
	cd ./tests/test_prime_cnt_cluster && docker compose down -v --remove-orphans

clean: test-clean
	rm -rf build/*