# remops Makefile — local build/deploy with version stamping.
#
# Release builds go through goreleaser (.goreleaser.yaml). This Makefile mirrors
# the same -X ldflags so LOCAL `go build`/`go install` produce git-stamped
# binaries instead of the bare "dev" default in main.go.

BINARY  := remops
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.date=$(DATE)

# fleet@OCI deploy target. Override on the command line, e.g.:
#   make deploy-oci OCI_HOST=oci OCI_BIN=/home/ubuntu/.hermes/bin/remops-bin
# OCI is ARM64 (Graviton). The remote binary name is remops-bin (Hermes MCP).
OCI_HOST ?= oci
OCI_BIN  ?= /home/ubuntu/.hermes/bin/remops-bin

.PHONY: build test vet version install-local deploy-oci clean

## build: compile a version-stamped binary for the current platform
build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

## test: race-enabled test suite
test:
	go test -race ./...

## vet: static checks
vet:
	go vet ./...

## version: print the version metadata that would be stamped
version:
	@echo "version=$(VERSION) commit=$(COMMIT) date=$(DATE)"

## install-local: install the personal@dev binary (operator + no-approver)
## Reminder: personal@dev runs `remops mcp --profile operator` with NO approval
## section in remops.yaml. Unsafe ops are denied -> deliberate escalation.
install-local:
	go install -ldflags "$(LDFLAGS)" .
	@echo "Installed $(BINARY) $(VERSION) to $$(go env GOPATH)/bin"
	@echo "personal@dev: ensure ~/.config/remops/remops.yaml has NO 'approval:' section."

## deploy-oci: cross-compile for OCI (linux/arm64) and copy the fleet binary.
## Restart is NOT automated: restarting Hermes risks disrupting co-located
## services (bluenode). See deploy/README.md for the deliberate restart path.
deploy-oci:
	mkdir -p dist
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-linux-arm64 .
	scp dist/$(BINARY)-linux-arm64 $(OCI_HOST):$(OCI_BIN)
	@echo "Copied $(BINARY) $(VERSION) -> $(OCI_HOST):$(OCI_BIN)"
	@echo "Now restart the MCP deliberately (see deploy/README.md) — do NOT blind-restart Hermes."

## clean: remove build artifacts
clean:
	rm -f $(BINARY)
	rm -rf dist
