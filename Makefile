.PHONY: help build run test deb e2e pre-pr clean

BIN := bin/joblet-flow

help:
	@echo "joblet-flow"
	@echo ""
	@echo "  make build     - Build the engine -> bin/joblet-flow"
	@echo "  make run       - Run the engine (mTLS to joblet from rnx-config.yml)"
	@echo "  make test      - Run unit tests"
	@echo "  make deb       - Build the joblet-flow .deb from the working tree"
	@echo "  make e2e       - Clean-room e2e: uninstall flow+joblet, install"
	@echo "                   latest released joblet, install flow from tree,"
	@echo "                   run the suites (needs sudo)"
	@echo "  make pre-pr    - Full pre-PR check (fmt, vet, tidy, tests, build, e2e)"
	@echo "  make clean     - Remove build artifacts"

build:
	go build -o $(BIN) ./cmd/joblet-flow

run: build
	./$(BIN)

test:
	@echo "Running tests (cache disabled)..."
	@go test -count=1 ./...
	@echo "✅ All tests complete"

deb:
	@./scripts/build-deb.sh

e2e:
	@./tests/e2e/run_tests.sh

pre-pr:
	@./scripts/pre-pr-check.sh

clean:
	rm -rf bin joblet-flow-deb-* joblet-flow_*.deb
