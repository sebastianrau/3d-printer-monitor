# Adding a messenger implementation

This guide describes every repository change required to add a messaging
provider. The example uses Slack; the same steps apply to Discord or another
service.

## 1. Understand the contracts

Implement `messenger.Service` from `pkg/messenger/messenger.go`:

```go
type Service interface {
    Messenger
    Validate() error
    Run(context.Context, []SnapshotSource)
}

type Messenger interface {
    SendImage(context.Context, []byte, string, string, string, int) error
    SendText(context.Context, string, string) error
}
```

The methods have these exact responsibilities:

- `Validate` checks whether normal monitoring can send messages, including the
  configured destination. It must not perform long-running network work.
- `SendImage` sends the in-memory image. Its arguments are context, JPEG bytes,
  filename, printer display name, milestone label, and progress percentage.
- `SendText` sends text to the supplied provider-specific destination ID. This
  is primarily used for command replies and discovery.
- `Run` owns optional incoming-command processing. It receives provider-neutral
  snapshot sources and should run until the context is cancelled. If the
  provider has no incoming commands, implement it as `<-ctx.Done()`.

Incoming commands must use only this neutral interface:

```go
type SnapshotSource interface {
    Name() string
    Identifier() string
    RequestManualSnapshot() bool
}
```

Do not import `printermonitor.Monitor` into the provider. `RequestManualSnapshot`
returns false when the monitor's event queue is full.

Target discovery is optional. A provider that supports it may additionally
implement:

```go
type TargetDiscoverer interface {
    FindTargets(context.Context, time.Duration, bool) (bool, error)
}
```

The current CLI discovery command uses this capability dynamically. Providers
without it still work for normal monitoring.

## 2. Add typed configuration

Add provider settings to `pkg/config/config.go`:

```go
type Messaging struct {
    Provider string         `yaml:"provider"`
    Telegram TelegramConfig `yaml:"telegram"`
    Slack    SlackConfig    `yaml:"slack"`
}

type SlackConfig struct {
    BotToken      string `yaml:"bot_token"`
    Channel       string `yaml:"channel"`
    TimeoutSeconds int   `yaml:"timeout_seconds"`
}
```

Add provider defaults to `Config.defaults`. Provider-specific construction
validation belongs in its registry factory; normal-send readiness belongs in
`Service.Validate`. This separation allows target discovery before a destination
has been configured.

Strict YAML decoding rejects fields that are not represented in these types.

## 3. Create the implementation package

Create `pkg/messenger/slack/client.go`:

```go
package slack

import (
    "context"
    "fmt"

    "github.com/sebastianrau/3d-printer-monitor/pkg/config"
    "github.com/sebastianrau/3d-printer-monitor/pkg/messenger"
)

var _ messenger.Service = (*Slack)(nil)

type Slack struct {
    config config.SlackConfig
    // HTTP/API client fields.
}

func New(c config.SlackConfig) (*Slack, error) {
    if c.BotToken == "" {
        return nil, fmt.Errorf("slack is missing bot_token")
    }
    return &Slack{config: c}, nil
}

func (s *Slack) Validate() error {
    if s.config.Channel == "" {
        return fmt.Errorf("slack is missing channel")
    }
    return nil
}

func (s *Slack) SendImage(
    ctx context.Context,
    image []byte,
    filename, printer, milestone string,
    progress int,
) error {
    // Upload image bytes and format the provider-specific caption.
    return nil
}

func (s *Slack) SendText(ctx context.Context, destination, text string) error {
    // Send text to destination.
    return nil
}

func (s *Slack) Run(ctx context.Context, sources []messenger.SnapshotSource) {
    // Poll/listen for commands, or wait if commands are unsupported.
    <-ctx.Done()
}
```

All requests must use the supplied context and enforce configured HTTP
timeouts. Keep authentication, API payloads, command parsing, authorization,
rate limiting, and retries owned by this package. The monitor already retries
the combined snapshot-and-delivery operation according to printer-level
delivery settings.

## 4. Register the factory

Import the package in `pkg/messenger/registry/registry.go` and register it inside
`New`:

```go
r.Register("slack", func(c config.Messaging) (messenger.Service, error) {
    return slack.New(c.Slack)
})
```

The key must match `messaging.provider`; the registry normalizes lowercase and
surrounding whitespace. Do not add provider construction or a provider switch
to `main.go`.

## 5. Document the YAML

Add every supported setting and default to `config.example.yaml`:

```yaml
messaging:
  provider: slack
  slack:
    bot_token: "replace-me"
    channel: "C0123456789"
    timeout_seconds: 30
```

Only the block selected by `messaging.provider` is used. Mark secrets clearly
and keep real tokens in ignored `config.yaml`.

## 6. Optional incoming commands and discovery

For commands, select sources by `Name()` or `Identifier()`, authorize the
provider-specific sender/destination, apply a cooldown, and call
`RequestManualSnapshot()`. Command replies use `SendText`.

If the provider can discover destination IDs, add the compile-time assertion
and method:

```go
var _ messenger.TargetDiscoverer = (*Slack)(nil)

func (s *Slack) FindTargets(
    ctx context.Context,
    wait time.Duration,
    includeOld bool,
) (bool, error) {
    // Wait for a provider-specific discovery command and report found IDs.
    return false, nil
}
```

Do not introduce provider-specific types into `cmd/3d-printer-monitor/main.go`.

## 7. Test the implementation

Add package tests covering at least:

- missing credentials and destination validation;
- image upload fields, filename, caption data, and raw image bytes;
- text destination and payload;
- non-success HTTP/API responses and malformed responses;
- timeouts and context cancellation;
- command authorization, selection, cooldown, and full queues;
- discovery when `TargetDiscoverer` is implemented;
- registry selection and unsupported providers;
- config defaults and strict-field rejection.

Use `httptest.Server` or an injected fake client; tests must not call the real
provider. Then run:

```bash
go test -race ./...
go vet ./...
go build ./cmd/3d-printer-monitor
```

Select the new provider in `config.yaml` and start normally. No change to
`cmd/3d-printer-monitor/main.go` is required.
