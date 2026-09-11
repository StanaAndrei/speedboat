.PHONY: build build-master-cli build-agent-cli clean

build: build-master-cli build-agent-cli


build-master-cli:
	go build -o build/speedboat-master ./cmd/master

build-agent-cli:
	go build -o build/speedboat-agent ./cmd/agent

clean:
	rm -rf build/*