package p1s

import (
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
		t.Fatal("unexpected P1S topics")
	}
}

func TestNewSelectsMJPEGCamera(t *testing.T) {
	p := New(config.Printer{Bambu: &config.BambuPrinter{}})
	if _, ok := p.camera.(*bambu.MJPEGCamera); !ok {
		t.Fatalf("camera type = %T", p.camera)
	}
}
