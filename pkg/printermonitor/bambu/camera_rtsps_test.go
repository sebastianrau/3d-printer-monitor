package bambu

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastianrau/3d-printer-monitor/pkg/config"
)

func TestCameraWarmupFilter(t *testing.T) {
	if got := cameraWarmupFilter(2); got != `select=gte(n\,2)` {
		t.Fatalf("camera filter = %q", got)
	}
	if got := cameraWarmupFilter(0); got != `select=gte(n\,0)` {
		t.Fatalf("zero-warmup camera filter = %q", got)
	}
}

func TestRTSPCameraUsesPortableFFmpegArguments(t *testing.T) {
	ffmpeg := filepath.Join(t.TempDir(), "ffmpeg")
	argsFile := filepath.Join(t.TempDir(), "args")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$FFMPEG_ARGS_FILE\"\nprintf '\\377\\330\\377\\331'\n"
	if err := os.WriteFile(ffmpeg, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(ffmpeg))
	t.Setenv("FFMPEG_ARGS_FILE", argsFile)

	camera := NewRTSPCamera(config.BambuPrinter{
		Host:                 "192.0.2.1",
		AccessCode:           "secret",
		CameraTimeoutSeconds: 5,
	})
	if _, err := camera.CaptureSnapshot(t.Context()); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(args), "tls_verify") {
		t.Fatalf("ffmpeg arguments contain unsupported tls_verify option: %s", args)
	}
	if !strings.Contains(string(args), "-i\nrtsps://bblp:secret@192.0.2.1:322/streaming/live/1\n") {
		t.Fatalf("ffmpeg arguments do not contain the RTSPS input: %s", args)
	}
}
