package bambu

import "testing"

func TestCameraWarmupFilter(t *testing.T) {
	if got := cameraWarmupFilter(2); got != `select=gte(n\,2)` {
		t.Fatalf("camera filter = %q", got)
	}
	if got := cameraWarmupFilter(0); got != `select=gte(n\,0)` {
		t.Fatalf("zero-warmup camera filter = %q", got)
	}
}
