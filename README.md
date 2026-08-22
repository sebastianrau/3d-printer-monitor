# 3d-printer-monitor

[![Quality](https://github.com/sebastianrau/3d-printer-monitor/actions/workflows/quality.yml/badge.svg)](https://github.com/sebastianrau/3d-printer-monitor/actions/workflows/quality.yml)

`3d-printer-monitor` monitors printers, creates in-memory camera snapshots at print
milestones, and sends notifications through a configurable messaging provider.
It is designed as a small framework: printer communication and messaging
services are replaceable without changing the monitor or the executable.

Only the Bambu model keys `p1` and `p1s` are currently supported. `p2s`, `x1`,
and `x1c` are rejected until dedicated implementations have been tested with
the corresponding hardware.

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
        └── p1s/          # direct P1/P1S implementation
```

`printermonitor.Printer` does not know about Bambu or MQTT. Implementations
provide lifecycle handling, normalized status reports, diagnostics, and
snapshots. The Bambu layer owns MQTT/TLS, reconnection, and delta-state
handling; the P1S package provides model-specific topics, report decoding, and
camera access. A future printer can use HTTP, serial, WebSocket, or another
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

A non-Bambu driver can own a block such as `octoprint:` or `serial:` without
placing its fields in the Bambu configuration. Unknown and obsolete YAML fields
are rejected during startup.

## Run

```bash
cp config.example.yaml config.yaml
go run ./cmd/3d-printer-monitor --config config.yaml
```

Do not commit real printer access codes or bot tokens.

## Tests and build

```bash
go test ./...
go vet ./...
go build ./cmd/3d-printer-monitor
docker build -t 3d-printer-monitor:local .
```

The Makefile can build the image and start the container with the local
`config.yaml` mounted read-only:

```bash
make docker-up
make docker-logs
```

A running Docker-compatible engine is required. On macOS, install and start
Docker Desktop, Colima, OrbStack, or another container runtime. Verify it before
building with `docker info`. The optional Docker Buildx plugin removes Docker's
legacy-builder warning but does not replace the required engine.

Use `make help` to list lifecycle targets. Image name, container name, and
configuration path can be overridden:

```bash
make docker-up IMAGE=registry.example/3d-printer-monitor:latest CONFIG=/path/to/config.yaml
```

Test a configured printer or discover a Telegram chat ID:

```bash
3d-printer-monitor --config config.yaml --test-printer "Office P1S" --test-timeout 10
3d-printer-monitor --config config.yaml --find-telegram-chat-id --telegram-wait 30
```

Camera images are held only in memory and passed directly to the messenger. No
image files are written to the device.

Milestone and duplicate-suppression state also remains in memory. On startup,
the first printer report becomes the baseline so an already running print does
not produce old milestone notifications. The application writes no runtime
state to the SD card.

Telegram accepts `/snapshot` and the compatibility alias `/snapshop`. Without a
selector, all printers take a snapshot. A printer name, exact identifier, or
identifier prefix selects a printer.

## License

This project is licensed under the [MIT License](LICENSE).
