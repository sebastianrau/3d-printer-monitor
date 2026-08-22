DOCKER ?= docker
IMAGE ?= 3d-printer-monitor:local
CONTAINER ?= 3d-printer-monitor
CONFIG ?= config.yaml
CONTAINER_CONFIG ?= /etc/3d-printer-monitor/config.yaml
HOST_UID ?= $(shell id -u)
HOST_GID ?= $(shell id -g)

.DEFAULT_GOAL := help

.PHONY: help test build check-docker docker-build docker-start docker-up docker-stop docker-restart docker-recreate docker-logs docker-status

help:
	@echo "Available targets:"
	@echo "  make test              Run Go tests"
	@echo "  make build             Build the local Go binary"
	@echo "  make docker-build      Build the Docker image"
	@echo "  make docker-start      Start a new container"
	@echo "  make docker-up         Build the image and start a new container"
	@echo "  make docker-stop       Stop the container"
	@echo "  make docker-restart    Restart the existing container"
	@echo "  make docker-recreate   Replace the container using the current image"
	@echo "  make docker-logs       Follow container logs"
	@echo "  make docker-status     Show container status"
	@echo ""
	@echo "Overrides: IMAGE=... CONTAINER=... CONFIG=..."

test:
	go test -race ./...

build:
	go build -o 3d-printer-monitor ./cmd/3d-printer-monitor

check-docker:
	@$(DOCKER) info >/dev/null 2>&1 || { \
		echo "Docker Engine is not reachable."; \
		echo "Start Docker Desktop, Colima, OrbStack, or another Docker daemon."; \
		echo "Then verify the selected context with: docker context ls"; \
		echo "Current context: $$($(DOCKER) context show 2>/dev/null || echo unknown)"; \
		exit 1; \
	}

docker-build: check-docker
	$(DOCKER) build --tag "$(IMAGE)" .

docker-start: check-docker check-config
	@if $(DOCKER) container inspect "$(CONTAINER)" >/dev/null 2>&1; then \
		echo "Container $(CONTAINER) already exists."; \
		echo "Use 'make docker-restart' or 'make docker-recreate'."; \
		exit 1; \
	fi
	$(DOCKER) run --detach \
		--name "$(CONTAINER)" \
		--restart unless-stopped \
		--user "$(HOST_UID):$(HOST_GID)" \
		--volume "$(abspath $(CONFIG)):$(CONTAINER_CONFIG):ro" \
		"$(IMAGE)"

docker-up:
	$(MAKE) docker-build
	$(MAKE) docker-start

docker-stop: check-docker
	$(DOCKER) stop "$(CONTAINER)"

docker-restart: check-docker
	$(DOCKER) restart "$(CONTAINER)"

docker-recreate: check-docker check-config
	@if $(DOCKER) container inspect "$(CONTAINER)" >/dev/null 2>&1; then \
		$(DOCKER) rm --force "$(CONTAINER)"; \
	fi
	$(MAKE) docker-start

docker-logs: check-docker
	$(DOCKER) logs --follow "$(CONTAINER)"

docker-status: check-docker
	$(DOCKER) ps --all --filter "name=^/$(CONTAINER)$$"

.PHONY: check-config
check-config:
	@test -f "$(CONFIG)" || { \
		echo "Configuration file not found: $(CONFIG)"; \
		echo "Copy config.example.yaml to $(CONFIG) and configure it first."; \
		exit 1; \
	}
