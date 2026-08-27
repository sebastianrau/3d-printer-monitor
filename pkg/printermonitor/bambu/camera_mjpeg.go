package bambu

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/sebastianrau/3d-printer-monitor/pkg/config"
)

// MJPEGCamera reads the framed JPEG stream exposed over TCP/TLS port 6000.
type MJPEGCamera struct{ settings config.BambuPrinter }

func NewMJPEGCamera(settings config.BambuPrinter) *MJPEGCamera {
	return &MJPEGCamera{settings: settings}
}

func (c *MJPEGCamera) CaptureSnapshot(ctx context.Context) ([]byte, error) {
	d := net.Dialer{Timeout: time.Duration(c.settings.CameraTimeoutSeconds) * time.Second}
	raw, err := d.DialContext(ctx, "tcp", net.JoinHostPort(c.settings.Host, "6000"))
	if err != nil {
		return nil, err
	}
	defer func() { _ = raw.Close() }()

	conn := tls.Client(raw, &tls.Config{InsecureSkipVerify: true})
	if err = conn.HandshakeContext(ctx); err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(time.Duration(c.settings.CameraTimeoutSeconds) * time.Second))

	auth := make([]byte, 80)
	binary.LittleEndian.PutUint32(auth[0:4], 0x40)
	binary.LittleEndian.PutUint32(auth[4:8], 0x3000)
	copy(auth[16:20], "bblp")
	code := []byte(c.settings.AccessCode)
	if len(code) > 31 {
		code = code[:31]
	}
	copy(auth[48:], code)
	if _, err = io.CopyN(conn, bytes.NewReader(auth), int64(len(auth))); err != nil {
		return nil, err
	}

	for valid := 0; ; {
		header := make([]byte, 16)
		if _, err = io.ReadFull(conn, header); err != nil {
			return nil, err
		}
		n := binary.LittleEndian.Uint32(header[:4])
		if n == 0 || n > 20*1024*1024 {
			return nil, fmt.Errorf("invalid camera payload size: %d", n)
		}
		frame := make([]byte, n)
		if _, err = io.ReadFull(conn, frame); err != nil {
			return nil, err
		}
		if isJPEG(frame) {
			if valid < c.settings.WarmupFrames() {
				valid++
				continue
			}
			return frame, nil
		}
	}
}

func isJPEG(b []byte) bool {
	return len(b) >= 4 && b[0] == 0xff && b[1] == 0xd8 && b[len(b)-2] == 0xff && b[len(b)-1] == 0xd9
}
