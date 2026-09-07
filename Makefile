.PHONY: help build run test pre-pr clean

BIN := bin/joblet-flow

help:
	@echo "joblet-flow"
	@echo ""
	@echo "  make build     - Build the engine -> bin/joblet-flow"
	@echo "  make run       - Run the engine (mTLS to joblet from rnx-config.yml)"
	@echo "  make test      - Run unit tests"
	@echo "  make pre-pr    - Full pre-PR check (fmt, vet, tidy, tests, build)"
	@echo "  make clean     - Remove build artifacts"
	@echo ""
	@echo "  Engine integration is tested end-to-end by joblet-flow-sdk-python's e2e."

build:
	go build -o $(BIN) ./cmd/joblet-flow

run: build
	./$(BIN)

test:
	go test ./...

pre-pr:
	@./scripts/pre-pr-check.sh

clean:
	rm -rf bin
