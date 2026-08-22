package p1s

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
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
		return fmt.Errorf("P1S camera: %w", err)
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

func (p *Printer) CaptureSnapshot(ctx context.Context) ([]byte, error) {
	d := net.Dialer{Timeout: time.Duration(p.Bambu.CameraTimeoutSeconds) * time.Second}
	raw, err := d.DialContext(ctx, "tcp", net.JoinHostPort(p.Bambu.Host, "6000"))
	if err != nil {
		return nil, err
	}
	defer func() { _ = raw.Close() }()
	c := tls.Client(raw, &tls.Config{InsecureSkipVerify: true})
	if err = c.HandshakeContext(ctx); err != nil {
		return nil, err
	}
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(time.Duration(p.Bambu.CameraTimeoutSeconds) * time.Second))
	auth := make([]byte, 80)
	binary.LittleEndian.PutUint32(auth[0:4], 0x40)
	binary.LittleEndian.PutUint32(auth[4:8], 0x3000)
	copy(auth[16:20], "bblp")
	code := []byte(p.Bambu.AccessCode)
	if len(code) > 31 {
		code = code[:31]
	}
	copy(auth[48:], code)
	if _, err = io.CopyN(c, bytesReader(auth), int64(len(auth))); err != nil {
		return nil, err
	}
	for valid := 0; ; {
		header := make([]byte, 16)
		if _, err = io.ReadFull(c, header); err != nil {
			return nil, err
		}
		n := binary.LittleEndian.Uint32(header[:4])
		if n == 0 || n > 20*1024*1024 {
			return nil, fmt.Errorf("invalid camera payload size: %d", n)
		}
		frame := make([]byte, n)
		if _, err = io.ReadFull(c, frame); err != nil {
			return nil, err
		}
		if len(frame) >= 4 && frame[0] == 0xff && frame[1] == 0xd8 && frame[len(frame)-2] == 0xff && frame[len(frame)-1] == 0xd9 {
			if valid < p.Bambu.WarmupFrames() {
				valid++
				continue
			}
			return frame, nil
		}
	}
}

type sliceReader struct{ b []byte }

func bytesReader(b []byte) *sliceReader { return &sliceReader{b: b} }
func (r *sliceReader) Read(p []byte) (int, error) {
	if len(r.b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.b)
	r.b = r.b[n:]
	return n, nil
}
