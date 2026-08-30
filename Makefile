DOCKER ?= docker
COMPOSE ?= $(DOCKER) compose
IMAGE ?= ghcr.io/sebastianrau/3d-printer-monitor:latest
CONTAINER ?= 3d-printer-monitor
CONFIG ?= ./config.yaml
HOST_UID ?= $(shell id -u)
HOST_GID ?= $(shell id -g)
BUILD_DIR ?= build

COMPOSE_ENV = IMAGE="$(IMAGE)" CONTAINER="$(CONTAINER)" CONFIG="$(abspath $(CONFIG))" HOST_UID="$(HOST_UID)" HOST_GID="$(HOST_GID)"

.DEFAULT_GOAL := help

.PHONY: help test build check-docker check-config docker-pull docker-up docker-stop docker-restart docker-recreate docker-logs docker-status

help:
	@echo "Available targets:"
	@echo "  make test              Run Go tests"
	@echo "  make build             Build the local Go binary"
	@echo "  make docker-pull       Pull the published multi-architecture image"
	@echo "  make docker-up         Pull and start the service"
	@echo "  make docker-stop       Stop and remove the service"
	@echo "  make docker-restart    Restart the service"
	@echo "  make docker-recreate   Pull and recreate the service"
	@echo "  make docker-logs       Follow service logs"
	@echo "  make docker-status     Show service status"
	@echo ""
	@echo "Overrides: IMAGE=... CONTAINER=... CONFIG=..."

test:
	go test -race ./...

build:
	mkdir -p "$(BUILD_DIR)"
	go build -o "$(BUILD_DIR)/3d-printer-monitor" ./cmd/3d-printer-monitor

check-docker:
	@$(DOCKER) info >/dev/null 2>&1 || { \
		echo "Docker Engine is not reachable."; \
		exit 1; \
	}
	@$(COMPOSE) version >/dev/null 2>&1 || { \
		echo "Docker Compose is not available."; \
		exit 1; \
	}

check-config:
	@test -f "$(CONFIG)" || { \
		echo "Configuration file not found: $(CONFIG)"; \
		echo "Copy config.example.yaml to $(CONFIG) and configure it first."; \
		exit 1; \
	}

docker-pull: check-docker
	$(COMPOSE_ENV) $(COMPOSE) pull

docker-up: check-docker check-config
	$(COMPOSE_ENV) $(COMPOSE) pull
	$(COMPOSE_ENV) $(COMPOSE) up --detach --remove-orphans

docker-stop: check-docker
	$(COMPOSE_ENV) $(COMPOSE) down

docker-restart: check-docker
	$(COMPOSE_ENV) $(COMPOSE) restart

docker-recreate: check-docker check-config
	$(COMPOSE_ENV) $(COMPOSE) pull
	$(COMPOSE_ENV) $(COMPOSE) up --detach --force-recreate --remove-orphans

docker-logs: check-docker
	$(COMPOSE_ENV) $(COMPOSE) logs --follow

docker-status: check-docker
	$(COMPOSE_ENV) $(COMPOSE) ps
