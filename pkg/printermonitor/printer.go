// Package printermonitor defines printer monitoring independent of transport.
package printermonitor

import (
	"context"
	"time"
)

type ReportHandler func(map[string]any)

// Printer is transport-neutral. Implementations may communicate through MQTT,
// HTTP, serial, WebSocket, or any other mechanism.
type Printer interface {
	Start(context.Context, ReportHandler) error
	Stop()
	CaptureSnapshot(context.Context) ([]byte, error)
	Diagnose(context.Context, time.Duration) error
}
