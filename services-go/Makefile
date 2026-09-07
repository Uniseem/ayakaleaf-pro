# Go ports of the Overleaf services. See README.md.

GO ?= go
BIN_DIR := bin
CMDS := chat notifications linked-url-proxy

.PHONY: all build test vet fmt lint clean conformance

all: build

build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/ ./cmd/...

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w .

# Fails if anything is unformatted, for CI.
lint:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "not gofmt'd:"; echo "$$unformatted"; exit 1; \
	fi
	$(GO) vet ./...

# Runs each Node service's own acceptance suite against the Go port.
# Needs a reachable MongoDB; see scripts/conformance.sh.
conformance:
	./scripts/conformance.sh all

clean:
	rm -rf $(BIN_DIR)
