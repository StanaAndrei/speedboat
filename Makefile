.PHONY: build build-master-cli build-agent-cli clean

build: build-master-cli build-agent-cli


build-master-cli:
	go build -o build/speedboat-master ./cmd/master

build-agent-cli:
	go build -o build/speedboat-agent ./cmd/agent

compose:
	cd ./examples && docker compose up -d

kill:
	cd ./examples && docker compose down

clean:
	rm -rf build/*