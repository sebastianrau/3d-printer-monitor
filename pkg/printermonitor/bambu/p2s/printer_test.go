package p2s

import (
	"encoding/json"
	"testing"

	"github.com/sebastianrau/3d-printer-monitor/pkg/config"
	"github.com/sebastianrau/3d-printer-monitor/pkg/printermonitor/bambu"
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

func TestNewSelectsRTSPCamera(t *testing.T) {
	p := New(config.Printer{Bambu: &config.BambuPrinter{}})
	if _, ok := p.camera.(*bambu.RTSPCamera); !ok {
		t.Fatalf("camera type = %T", p.camera)
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
