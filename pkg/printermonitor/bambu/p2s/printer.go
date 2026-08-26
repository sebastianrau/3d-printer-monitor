// Package p2s implements Bambu Lab P2S printer communication.
package p2s

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"strings"
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
}

func New(c config.Printer) *Printer {
	settings := c.BambuSettings()
	p := &Printer{Config: c, Bambu: settings}
	p.connection = bambu.NewMQTTConnection(c, settings, protocol{serial: settings.Serial})
	return p
}

func (p *Printer) Start(ctx context.Context, handler printermonitor.ReportHandler) error {
	return p.connection.Start(ctx, handler)
}

func (p *Printer) Stop() { p.connection.Stop() }

func (p *Printer) Diagnose(ctx context.Context, timeout time.Duration) error {
	reportReceived := make(chan struct{}, 1)
	connectionCtx, cancelConnection := context.WithTimeout(ctx, timeout)
	if err := p.Start(connectionCtx, func(map[string]any) {
		select {
		case reportReceived <- struct{}{}:
		default:
		}
	}); err != nil {
		cancelConnection()
		return fmt.Errorf("bambu MQTT connection: %w", err)
	}
	select {
	case <-reportReceived:
		p.Stop()
		cancelConnection()
	case <-connectionCtx.Done():
		p.Stop()
		cancelConnection()
		return fmt.Errorf("bambu status report: %w", connectionCtx.Err())
	}

	cameraCtx, cancelCamera := context.WithTimeout(ctx, timeout)
	defer cancelCamera()
	if _, err := p.CaptureSnapshot(cameraCtx); err != nil {
		return fmt.Errorf("P2S camera: %w", err)
	}
	return nil
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

// CaptureSnapshot reads one frame from the P2S RTSPS/H.264 camera and converts
// it to JPEG. ffmpeg is used because the Go standard library has no H.264
// decoder; the project container image includes it.
func (p *Printer) CaptureSnapshot(ctx context.Context) ([]byte, error) {
	cameraCtx, cancel := context.WithTimeout(ctx, time.Duration(p.Bambu.CameraTimeoutSeconds)*time.Second)
	defer cancel()

	stream := &url.URL{
		Scheme: "rtsps",
		User:   url.UserPassword("bblp", p.Bambu.AccessCode),
		Host:   net.JoinHostPort(p.Bambu.Host, "322"),
		Path:   "/streaming/live/1",
	}
	streamURL := stream.String()
	cmd := exec.CommandContext(cameraCtx, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-rtsp_transport", "tcp", "-i", streamURL,
		"-vf", cameraWarmupFilter(p.Bambu.WarmupFrames()),
		"-frames:v", "1", "-f", "image2pipe", "-c:v", "mjpeg", "pipe:1",
	)
	var image bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &image
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if cameraCtx.Err() != nil {
			return nil, cameraCtx.Err()
		}
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			redactedStream := *stream
			redactedStream.User = url.UserPassword("bblp", "REDACTED")
			detail = strings.ReplaceAll(detail, streamURL, redactedStream.String())
			return nil, fmt.Errorf("ffmpeg: %w: %s", err, detail)
		}
		return nil, fmt.Errorf("ffmpeg: %w", err)
	}
	b := image.Bytes()
	if len(b) < 4 || b[0] != 0xff || b[1] != 0xd8 || b[len(b)-2] != 0xff || b[len(b)-1] != 0xd9 {
		return nil, fmt.Errorf("camera returned an invalid JPEG frame")
	}
	return append([]byte(nil), b...), nil
}

func cameraWarmupFilter(frames int) string {
	return fmt.Sprintf("select=gte(n\\,%d)", frames)
}
