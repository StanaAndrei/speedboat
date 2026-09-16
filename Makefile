.PHONY: build build-master-cli build-agent-cli clean

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
test: test-cluster

test-cluster: test-prime-cnt-cluster

test-prime-cnt-cluster: build
	./tests/test_prime_cnt_cluster/test.sh

test-clean:
	cd ./tests/test_prime_cnt_cluster && docker compose down -v --remove-orphans

clean: test-clean
	rm -rf build/*