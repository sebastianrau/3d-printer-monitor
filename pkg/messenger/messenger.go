package messenger

import (
	"context"
	"time"
)

type Messenger interface {
	SendImage(context.Context, []byte, string, string, string, int) error
	SendText(context.Context, string, string) error
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
