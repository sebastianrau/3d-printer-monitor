#!/bin/sh

set -eu

IMAGE_REPOSITORY=${IMAGE_REPOSITORY:-ghcr.io/sebastianrau/3d-printer-monitor}
IMAGE_TAG=${IMAGE_TAG:-latest}
IMAGE=${IMAGE:-${IMAGE_REPOSITORY}:${IMAGE_TAG}}
CONTAINER_NAME=${CONTAINER_NAME:-3d-printer-monitor}
CONFIG_URL=${CONFIG_URL:-https://raw.githubusercontent.com/sebastianrau/3d-printer-monitor/main/config.example.yaml}

say() {
	printf '%s\n' "$*"
}

fail() {
	printf 'Error: %s\n' "$*" >&2
	exit 1
}

require_command() {
	command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

require_command docker
require_command curl

if [ -z "${INSTALL_DIR:-}" ]; then
	[ -n "${HOME:-}" ] || fail "HOME is not set; provide INSTALL_DIR explicitly"
	INSTALL_DIR=${HOME}/3d-printer-monitor
fi

docker info >/dev/null 2>&1 || fail "Docker Engine is not reachable. Start Docker or check your permissions."

mkdir -p "$INSTALL_DIR"
INSTALL_DIR=$(cd "$INSTALL_DIR" && pwd -P)

if [ -z "${CONFIG_PATH:-}" ]; then
	CONFIG_PATH=${INSTALL_DIR}/config.yaml
else
	case $CONFIG_PATH in
		/*) ;;
		*) CONFIG_PATH=$(pwd -P)/${CONFIG_PATH#./} ;;
	esac
fi

mkdir -p "$(dirname "$CONFIG_PATH")"

if [ ! -f "$CONFIG_PATH" ]; then
	temporary_config=${CONFIG_PATH}.tmp.$$
	trap 'rm -f "$temporary_config"' 0 HUP INT TERM
	curl --fail --silent --show-error --location \
		--output "$temporary_config" "$CONFIG_URL"
	chmod 600 "$temporary_config"
	mv "$temporary_config" "$CONFIG_PATH"
	trap - 0 HUP INT TERM

	say "Created $CONFIG_PATH"
	say "Edit the printer and Telegram settings, then run this installer again."
	exit 0
fi

chmod 600 "$CONFIG_PATH"

if grep -Eq 'AAExampleBotToken|192\.168\.192\.50|01P00A123456789|12345678|IP_ADDRESS_OF_|SERIAL_OF_|ACCESS_CODE_OF_' "$CONFIG_PATH"; then
	fail "placeholder credentials remain in $CONFIG_PATH; edit it or run with CONFIG_PATH=/path/to/config.yaml"
fi

say "Pulling $IMAGE"
docker pull "$IMAGE"

if docker container inspect "$CONTAINER_NAME" >/dev/null 2>&1; then
	say "Replacing existing container $CONTAINER_NAME"
	docker stop "$CONTAINER_NAME" >/dev/null
	docker rm "$CONTAINER_NAME" >/dev/null
fi

say "Starting $CONTAINER_NAME"
docker run --detach \
	--name "$CONTAINER_NAME" \
	--restart unless-stopped \
	--user "$(id -u):$(id -g)" \
	--volume "$CONFIG_PATH:/etc/3d-printer-monitor/config.yaml:ro" \
	--volume /etc/localtime:/etc/localtime:ro \
	"$IMAGE" >/dev/null

sleep 2

if [ "$(docker inspect --format '{{.State.Running}}' "$CONTAINER_NAME")" != "true" ]; then
	docker logs "$CONTAINER_NAME" >&2 || true
	fail "container exited during startup"
fi

say "3d-printer-monitor is running."
say "Configuration: $CONFIG_PATH"
say "Image:         $IMAGE"
say "Logs:          docker logs --follow $CONTAINER_NAME"
