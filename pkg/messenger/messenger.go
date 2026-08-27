package messenger

import (
	"context"
	"time"
)

type Messenger interface {
	SendImage(context.Context, []byte, string, string, string, int) error
	SendText(context.Context, string, string) error
}

// PrintStatus contains implementation-neutral fields for one print-job update.
type PrintStatus struct {
	Printer, Job, State, Stage string
	Progress                   *int
	Layer, TotalLayers         *int
	RemainingMinutes           *int
	NozzleTemperature          *float64
	BedTemperature             *float64
}

// ProgressMessenger is an optional capability for editable print-status messages.
type ProgressMessenger interface {
	PublishPrintStatus(context.Context, string, PrintStatus, bool, bool) error
}

// SnapshotSource is the provider-neutral view used by messenger commands.
type SnapshotSource interface {
	Name() string
	Identifier() string
	RequestManualSnapshot() bool
}

// Service is a complete messaging implementation. Run handles optional inbound
// commands and must block until the context is cancelled.
type Service interface {
	Messenger
	Validate() error
	Run(context.Context, []SnapshotSource)
}

// TargetDiscoverer is implemented by providers that can discover destination IDs.
type TargetDiscoverer interface {
	FindTargets(context.Context, time.Duration, bool) (bool, error)
}
