BUILD_DIR ?= build
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.DEFAULT_GOAL := help

.PHONY: help test vet build check

help:
	@echo "Available targets:"
	@echo "  make test              Run Go tests with the race detector"
	@echo "  make vet               Run Go static analysis"
	@echo "  make build             Build the local Go binary"
	@echo "  make check             Run vet, tests, and build"

test:
	go test -race ./...

vet:
	go vet ./...

build:
	mkdir -p "$(BUILD_DIR)"
	go build -trimpath -ldflags="$(LDFLAGS)" -o "$(BUILD_DIR)/3d-printer-monitor" ./cmd/3d-printer-monitor

check: vet test build
