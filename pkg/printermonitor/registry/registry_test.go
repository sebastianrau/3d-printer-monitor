package registry

import (
	"testing"

	"github.com/sebastianrau/3d-printer-monitor/pkg/config"
	"github.com/sebastianrau/3d-printer-monitor/pkg/printermonitor/bambu/p2s"
)

func TestCreatesP2SImplementation(t *testing.T) {
	c := config.Printer{
		Type: "bambu",
		Bambu: &config.BambuPrinter{
			Model:      "p2s",
			Host:       "192.0.2.1",
			Serial:     "P2SERIAL",
			AccessCode: "CODE",
		},
	}
	printer, err := New().Create(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := printer.(*p2s.Printer); !ok {
		t.Fatalf("printer type = %T", printer)
	}
}
