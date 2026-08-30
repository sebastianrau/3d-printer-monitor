# 3d-printer-monitor

[![Quality](https://github.com/sebastianrau/3d-printer-monitor/actions/workflows/quality.yml/badge.svg)](https://github.com/sebastianrau/3d-printer-monitor/actions/workflows/quality.yml)

`3d-printer-monitor` monitors printers, creates in-memory camera snapshots at print
milestones, and sends notifications through a configurable messaging provider.
It is designed as a small framework: printer communication and messaging
services are replaceable without changing the monitor or the executable.

The Bambu model keys `p1`, `p1s`, and `p2s` are supported. `x1` and `x1c` are
rejected until dedicated implementations have been tested with the
corresponding hardware.

## Framework structure

```text
pkg/
├── messenger/
│   ├── messenger.go      # provider-neutral interfaces
│   ├── registry/         # provider key → implementation
│   └── telegram/         # Telegram Bot API and commands
└── printermonitor/
    ├── printer.go        # transport-neutral Printer interface
    ├── monitor.go        # state machine and event queue
    ├── registry/         # printer key → implementation
    └── bambu/
        ├── mqtt.go       # shared Bambu MQTT/TLS transport
        ├── camera_mjpeg.go # shared TCP/TLS MJPEG camera transport
        ├── camera_rtsps.go # shared RTSPS/H.264 camera transport
        ├── p1s/          # direct P1/P1S implementation
        └── p2s/          # P2S MQTT and RTSPS camera implementation
```

`printermonitor.Printer` does not know about Bambu or MQTT. Implementations
provide lifecycle handling, normalized status reports, diagnostics, and
snapshots. The Bambu layer owns MQTT/TLS, reconnection, and delta-state
handling; the P1S package provides model-specific topics, report decoding, and
camera access. The P2S package uses the same MQTT status flow and captures its
RTSPS/H.264 camera through `ffmpeg`. A future printer can use HTTP, serial, WebSocket, or another
transport without changing the monitor or `Printer` interface.

Both extension points follow the same pattern: put the implementation in its
own package, provide a factory returning `(Implementation, error)`, and register
that factory with `Register(key, factory)`. `Create(config)` selects it at
runtime. `cmd/3d-printer-monitor/main.go` does not change.

- [Adding a printer implementation](docs/ADDING_A_PRINTER.md)
- [Adding a messenger implementation](docs/ADDING_A_MESSENGER.md)
- [Installing on Raspberry Pi](docs/RASPBERRY_PI_INSTALLATION.md)

Generic monitor options remain at printer level. Vendor- and transport-specific
settings live in a typed nested block:

```yaml
printers:
  - name: Office P1S
    type: bambu
    enabled: true
    event_queue_size: 16
    delivery_attempts: 3
    bambu:
      model: p1s
      host: 192.168.1.50
      serial: "..."
      access_code: "..."
```

For a P2S, set `model: p2s`. P2S camera snapshots use `ffmpeg`, which is already
installed in the project Docker image; the host does not need it.
`camera_warmup_frames` controls how many decoded frames are discarded before
capturing the snapshot.

A non-Bambu driver can own a block such as `octoprint:` or `serial:` without
placing its fields in the Bambu configuration. Unknown and obsolete YAML fields
are rejected during startup.

## Install and run

Released container images are published at
`ghcr.io/sebastianrau/3d-printer-monitor`. A multi-architecture manifest lets
Docker automatically select `linux/amd64`, `linux/arm64`, or `linux/arm/v7`.
The installer creates `~/3d-printer-monitor/config.yaml` on its first run. Edit
that file, then run the same command again to pull and start the container:

```bash
curl -fsSL https://raw.githubusercontent.com/sebastianrau/3d-printer-monitor/main/install.sh | sh
```

It never overwrites an existing configuration. To inspect the script before
running it, download it instead:

```bash
curl -fsSLO https://raw.githubusercontent.com/sebastianrau/3d-printer-monitor/main/install.sh
less install.sh
sh install.sh
```

Use an existing configuration from another location with:

```bash
CONFIG_PATH=/path/to/config.yaml sh install.sh
```

A running Docker-compatible engine is required. Docker Compose is not needed.
On macOS, install and start Docker Desktop, Colima, OrbStack, or another
container runtime and verify it with `docker info`.

The same installer command updates an existing installation. It pulls the
current image and replaces only the application container without overwriting
`config.yaml`.

Do not commit real printer access codes or bot tokens. See the
[Raspberry Pi installation guide](docs/RASPBERRY_PI_INSTALLATION.md) for the
complete host setup.

## Development

Local source builds are needed only when changing the application itself:

```bash
make check
```

This runs `go vet ./...`, race-enabled tests, and the local binary build. Normal
installation and updates do not require Git, Go, Make, or Docker Compose.

GitHub releases publish version, major/minor, major, commit-SHA, and `latest`
tags. Publishing uses the repository `GITHUB_TOKEN`; no registry secret is
required. After the first publication, set the GHCR package visibility to
public in its GitHub package settings to allow anonymous pulls.

Test a configured printer or discover a Telegram chat ID:

```bash
mkdir -p "$HOME/3d-printer-monitor/diagnostics"
docker run --rm \
  --user "$(id -u):$(id -g)" \
  --volume "$HOME/3d-printer-monitor/config.yaml:/etc/3d-printer-monitor/config.yaml:ro" \
  --volume "$HOME/3d-printer-monitor/diagnostics:/diagnostics" \
  --workdir /diagnostics \
  ghcr.io/sebastianrau/3d-printer-monitor:latest \
  --config /etc/3d-printer-monitor/config.yaml \
  --test-printer "Office P1S" --test-timeout 10
```

The printer diagnostic stores its captured image in
`~/3d-printer-monitor/diagnostics` as `<printer-name>-diagnostic.jpg`.

Only one process may poll a Telegram bot. Stop the normal monitor before
discovering the chat ID, send `/id` to the bot during the wait period, then run
the installer again afterward:

```bash
docker stop 3d-printer-monitor 2>/dev/null || true
docker run --rm \
  --user "$(id -u):$(id -g)" \
  --volume "$HOME/3d-printer-monitor/config.yaml:/etc/3d-printer-monitor/config.yaml:ro" \
  ghcr.io/sebastianrau/3d-printer-monitor:latest \
  --config /etc/3d-printer-monitor/config.yaml \
  --find-telegram-chat-id --telegram-wait 30
curl -fsSL https://raw.githubusercontent.com/sebastianrau/3d-printer-monitor/main/install.sh | sh
```

During normal monitoring, camera images are held only in memory and passed
directly to the messenger; no image files are written to the device.

Milestone and duplicate-suppression state also remains in memory. On startup,
the first printer report becomes the baseline so an already running print does
not produce old milestone notifications. The application writes no runtime
state to the SD card.

Telegram accepts `/snapshot` and the compatibility alias `/snapshop`. Without a
selector, all printers take a snapshot. A printer name, exact identifier, or
identifier prefix selects a printer.

For each newly detected print job, Telegram creates one editable status message
with a 10-segment emoji progress bar. Changed progress details are updated at
most once every 60 seconds. State transitions such as pause, resume, completion,
abort, or failure bypass that interval. Completed and aborted jobs receive one
final update and are then removed from the in-memory update tracker. When Bambu
MQTT supplies `stg_cur`, known printer operations such as `Heating bed` or
`Cleaning nozzle tip` are included in the same status message. If the monitor
starts while a job is already active, it creates a replacement status message
from the first complete MQTT report and continues updating that message. When
remaining time is available, the message also shows the estimated completion
time calculated in the process's local timezone.

## License

This project is licensed under the [MIT License](LICENSE).
