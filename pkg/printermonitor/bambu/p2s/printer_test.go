package p2s

import (
	"encoding/json"
	"testing"
)

func TestProtocolMergesDeltaReports(t *testing.T) {
	p := protocol{serial: "ABC"}
	state := map[string]any{}
	if _, err := p.DecodeReport([]byte(`{"print":{"gcode_state":"RUNNING","mc_percent":10}}`), state); err != nil {
		t.Fatal(err)
	}
	report, err := p.DecodeReport([]byte(`{"print":{"mc_percent":11}}`), state)
	if err != nil {
		t.Fatal(err)
	}
	if report["gcode_state"] != "RUNNING" || report["mc_percent"] != float64(11) {
		t.Fatalf("merged report = %#v", report)
	}
	if p.ReportTopic() != "device/ABC/report" || p.RequestTopic() != "device/ABC/request" {
		t.Fatal("unexpected P2S topics")
	}
}

func TestProtocolRequestsFullStatus(t *testing.T) {
	var request map[string]map[string]any
	if err := json.Unmarshal(protocol{}.InitialRequest(), &request); err != nil {
		t.Fatal(err)
	}
	if request["pushing"]["command"] != "pushall" || request["pushing"]["version"] != float64(1) {
		t.Fatalf("initial request = %#v", request)
	}
}

func TestCameraWarmupFilter(t *testing.T) {
	if got := cameraWarmupFilter(2); got != `select=gte(n\,2)` {
		t.Fatalf("camera filter = %q", got)
	}
	if got := cameraWarmupFilter(0); got != `select=gte(n\,0)` {
		t.Fatalf("zero-warmup camera filter = %q", got)
	}
}
