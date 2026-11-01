GO ?= go
VERSION ?= 0.2.0
BIN_DIR ?= bin
IMAGE_PREFIX ?= subgate
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: help build build-core build-agent build-worker build-ndpi test test-race vet lint check smoke demo docker bench clean

help:
	@echo "build  Build Core and Agent"
	@echo "test   Run tests; test-race runs the race detector; check runs all gates"
	@echo "docker Build both container images; demo runs a local demo"
build: build-core build-agent
build-core:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/subgate-core ./core/cmd
build-agent:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/subgate-agent ./agent/cmd
build-worker:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build -trimpath -o $(BIN_DIR)/subgate-worker ./agent/worker
build-ndpi:
	$(MAKE) -C agent/native
test:
	$(GO) test ./...
test-race:
	$(GO) test -race ./...
vet:
	$(GO) vet ./...
lint:
	@test -z "$$(gofmt -l core agent pkg web)"
	node --check web/app.js
	bash -n deploy/install.sh
	bash -n deploy/install-agent.sh
	bash scripts/test-agent-installer.sh
check: test test-race vet lint build
smoke: build
	python3 scripts/validate-linux.py --bin-dir $(BIN_DIR) --stats-address 127.0.0.1:1
demo:
	$(GO) run ./core/cmd -demo
docker:
	docker build -f deploy/Dockerfile.core --build-arg VERSION=$(VERSION) -t $(IMAGE_PREFIX)-core:$(VERSION) .
	docker build -f deploy/Dockerfile.agent --build-arg VERSION=$(VERSION) -t $(IMAGE_PREFIX)-agent:$(VERSION) .
bench:
	$(GO) test -run '^$$' -bench . -benchmem ./...
clean:
	rm -f $(BIN_DIR)/subgate-core $(BIN_DIR)/subgate-agent coverage.out coverage.html
