# Adding a printer implementation

This guide describes every repository change required to add a printer. The
example uses an `octoprint` implementation. Replace that name and its fields
with the real driver.

## 1. Understand the contract

Implement `printermonitor.Printer` from `pkg/printermonitor/printer.go`:

```go
type Printer interface {
    Start(context.Context, ReportHandler) error
    Stop()
    CaptureSnapshot(context.Context) ([]byte, error)
    Diagnose(context.Context, time.Duration) error
}
```

The methods have these exact responsibilities:

- `Start` establishes the connection, starts asynchronous status delivery, and
  calls the handler for every complete normalized report. Return only startup
  errors; reconnects after successful startup belong to the implementation.
- `Stop` stops background work and closes connections. It must be safe after a
  successful `Start` and should be idempotent.
- `CaptureSnapshot` returns complete JPEG bytes in memory. It must honor context
  cancellation and must not persist the image.
- `Diagnose` verifies both status communication and snapshot capture within the
  supplied timeout. Transport-specific checks belong here.

Each report is `map[string]any`. The monitor recognizes these keys:

| Key | Expected value | Purpose |
| --- | --- | --- |
| `gcode_state` | string | `IDLE`, `PREPARE`, `RUNNING`, `PAUSE`, `FINISH`, or `FAILED` |
| `mc_percent` | integer-compatible | Print progress from 0 to 100 |
| `layer_num` | integer-compatible | Current layer |
| `total_layer_num` | integer-compatible | Total layer count |
| `task_id` | string | Preferred stable print-job identity |
| `subtask_id` | string | Fallback job identity |
| `gcode_file` | string | Fallback job identity |
| `subtask_name` | string | Final fallback job identity |

At least one of `gcode_state`, `mc_percent`, or `layer_num` must be present for a
report to affect monitoring. Supply a stable job identity whenever possible so
new prints can be distinguished reliably.

## 2. Add typed configuration

Add the implementation-specific config and a pointer to it in
`pkg/config/config.go`:

```go
type Printer struct {
    // Existing generic fields...
    OctoPrint *OctoPrintPrinter `yaml:"octoprint"`
}

type OctoPrintPrinter struct {
    BaseURL string `yaml:"base_url"`
    APIKey  string `yaml:"api_key"`
}
```

Use a pointer so the block can be distinguished from an omitted block. Add a
typed accessor if the implementation needs one:

```go
func (p Printer) OctoPrintSettings() OctoPrintPrinter {
    if p.OctoPrint != nil {
        return *p.OctoPrint
    }
    return OctoPrintPrinter{}
}
```

Update the following config behavior in the same file:

1. `Config.defaults`: normalize the type and apply safe defaults only when
   `p.Type == "octoprint"`.
2. `Config.Validate`: allow `octoprint`, require its mandatory fields, and
   validate numeric ranges.
3. `Printer.Identifier`: return the implementation's stable identifier when the
   generic `id` is absent.
4. `Printer.RegistryKey`: return `octoprint`, or `octoprint/<model>` if multiple
   model implementations require separate factories.

Because YAML decoding uses `KnownFields(true)`, forgetting the typed field makes
the new YAML block fail immediately. Do not use an untyped `map[string]any` for
provider settings.

## 3. Create the implementation package

Create `pkg/printermonitor/octoprint/printer.go`:

```go
package octoprint

import (
    "context"
    "time"

    "github.com/sebastianrau/3d-printer-monitor/pkg/config"
    "github.com/sebastianrau/3d-printer-monitor/pkg/printermonitor"
)

var _ printermonitor.Printer = (*Printer)(nil)

type Printer struct {
    config config.OctoPrintPrinter
    // Client, cancellation, and synchronization fields.
}

func New(c config.Printer) (*Printer, error) {
    settings := c.OctoPrintSettings()
    // Construct clients here. Return configuration/construction errors.
    return &Printer{config: settings}, nil
}

func (p *Printer) Start(ctx context.Context, handler printermonitor.ReportHandler) error {
    // Connect and translate native status into the normalized report keys.
    return nil
}

func (p *Printer) Stop() {}

func (p *Printer) CaptureSnapshot(ctx context.Context) ([]byte, error) {
    panic("implement JPEG capture")
}

func (p *Printer) Diagnose(ctx context.Context, timeout time.Duration) error {
    panic("implement status and camera diagnostics")
}
```

Keep protocol, transport, authentication, reconnection, and native-to-normalized
status conversion inside this package or its vendor abstraction. Do not add
transport logic to `printermonitor.Monitor`.

## 4. Register the factory

Import the package in `pkg/printermonitor/registry/registry.go` and register it
inside `New`:

```go
r.Register("octoprint", func(c config.Printer) (printermonitor.Printer, error) {
    return octoprint.New(c)
})
```

The key must exactly match `Printer.RegistryKey()` after lowercase/whitespace
normalization. Do not add a switch or constructor to `main.go`.

## 5. Document the YAML

Add every supported option and its default to `config.example.yaml`:

```yaml
printers:
  - name: Workshop printer
    id: workshop
    type: octoprint
    octoprint:
      base_url: "https://octoprint.example"
      api_key: "replace-me"
```

Mark secrets clearly and keep real credentials only in ignored `config.yaml`.

## 6. Test the implementation

Add package tests covering at least:

- native status conversion into every normalized report field;
- startup failure and context cancellation;
- reconnect behavior, if supported;
- snapshot success, invalid payload, timeout, and cancellation;
- diagnostic success and each failed diagnostic stage;
- factory creation and unsupported registry keys;
- config defaults, validation, and rejection of unknown fields.

Use `httptest.Server`, fake transports, or injected clients. Tests must not need
real printer hardware. Then run:

```bash
go test -race ./...
go vet ./...
go build ./cmd/3d-printer-monitor
```

Finally, use the real-device diagnostic path:

```bash
3d-printer-monitor --config config.yaml --test-printer "Workshop printer" --test-timeout 10
```

No change to `cmd/3d-printer-monitor/main.go` is required.
