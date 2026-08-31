package bambu

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/sebastianrau/3d-printer-monitor/pkg/config"
)

// RTSPCamera reads the RTSPS/H.264 stream on port 322 and uses ffmpeg to
// decode a frame to JPEG.
type RTSPCamera struct{ settings config.BambuPrinter }

func NewRTSPCamera(settings config.BambuPrinter) *RTSPCamera {
	return &RTSPCamera{settings: settings}
}

func (c *RTSPCamera) CaptureSnapshot(ctx context.Context) ([]byte, error) {
	cameraCtx, cancel := context.WithTimeout(ctx, time.Duration(c.settings.CameraTimeoutSeconds)*time.Second)
	defer cancel()

	stream := &url.URL{
		Scheme: "rtsps",
		User:   url.UserPassword("bblp", c.settings.AccessCode),
		Host:   net.JoinHostPort(c.settings.Host, "322"),
		Path:   "/streaming/live/1",
	}
	streamURL := stream.String()
	cmd := exec.CommandContext(cameraCtx, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-rtsp_transport", "tcp",
		// FFmpeg does not verify TLS peer certificates by default. Do not pass
		// -tls_verify: some packaged FFmpeg builds do not expose that option.
		"-i", streamURL,
		"-vf", cameraWarmupFilter(c.settings.WarmupFrames()),
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
	if !isJPEG(b) {
		return nil, fmt.Errorf("camera returned an invalid JPEG frame")
	}
	return append([]byte(nil), b...), nil
}

func cameraWarmupFilter(frames int) string {
	return fmt.Sprintf("select=gte(n\\,%d)", frames)
}
