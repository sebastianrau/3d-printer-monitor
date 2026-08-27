package p1s

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sebastianrau/3d-printer-monitor/pkg/config"
	"github.com/sebastianrau/3d-printer-monitor/pkg/printermonitor"
	"github.com/sebastianrau/3d-printer-monitor/pkg/printermonitor/bambu"
)

var _ printermonitor.Printer = (*Printer)(nil)

type Printer struct {
	Config     config.Printer
	Bambu      config.BambuPrinter
	connection *bambu.MQTTConnection
	camera     bambu.Camera
}

func New(c config.Printer) *Printer {
	settings := c.BambuSettings()
	p := &Printer{Config: c, Bambu: settings}
	p.connection = bambu.NewMQTTConnection(c, settings, protocol{serial: settings.Serial})
	p.camera = bambu.NewMJPEGCamera(settings)
	return p
}

func (p *Printer) Start(ctx context.Context, handler printermonitor.ReportHandler) error {
	return p.connection.Start(ctx, handler)
}

func (p *Printer) Stop() { p.connection.Stop() }

func (p *Printer) Diagnose(ctx context.Context, timeout time.Duration) ([]byte, error) {
	reportReceived := make(chan struct{}, 1)
	connectionCtx, cancelConnection := context.WithTimeout(ctx, timeout)
	if err := p.Start(connectionCtx, func(map[string]any) {
		select {
		case reportReceived <- struct{}{}:
		default:
		}
	}); err != nil {
		cancelConnection()
		return nil, fmt.Errorf("bambu MQTT connection: %w", err)
	}
	select {
	case <-reportReceived:
		p.Stop()
		cancelConnection()
	case <-connectionCtx.Done():
		p.Stop()
		cancelConnection()
		return nil, fmt.Errorf("bambu status report: %w", connectionCtx.Err())
	}

	cameraCtx, cancelCamera := context.WithTimeout(ctx, timeout)
	defer cancelCamera()
	image, err := p.CaptureSnapshot(cameraCtx)
	if err != nil {
		return nil, fmt.Errorf("P1S camera: %w", err)
	}
	return image, nil
}

type protocol struct{ serial string }

func (p protocol) ReportTopic() string  { return "device/" + p.serial + "/report" }
func (p protocol) RequestTopic() string { return "device/" + p.serial + "/request" }
func (p protocol) InitialRequest() []byte {
	b, _ := json.Marshal(map[string]any{"pushing": map[string]any{"sequence_id": "0", "command": "pushall", "version": 1, "push_target": 1}})
	return b
}
func (p protocol) DecodeReport(payload []byte, accumulated map[string]any) (map[string]any, error) {
	var report map[string]any
	if err := json.Unmarshal(payload, &report); err != nil {
		return nil, err
	}
	deepMerge(accumulated, report)
	v, _ := accumulated["print"].(map[string]any)
	if stage := bambu.StageName(v["stg_cur"]); stage != "" {
		v["stage_name"] = stage
	} else {
		delete(v, "stage_name")
	}
	return v, nil
}
func deepMerge(dst, src map[string]any) {
	for k, v := range src {
		sm, ok := v.(map[string]any)
		if !ok {
			dst[k] = v
			continue
		}
		dm, _ := dst[k].(map[string]any)
		if dm == nil {
			dm = map[string]any{}
			dst[k] = dm
		}
		deepMerge(dm, sm)
	}
}

func (p *Printer) CaptureSnapshot(ctx context.Context) ([]byte, error) {
	return p.camera.CaptureSnapshot(ctx)
}
