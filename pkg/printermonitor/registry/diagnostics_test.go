package registry

import (
	"testing"
)

func TestDiagnosticSnapshotPath(t *testing.T) {
	tests := map[string]string{
		"P2S_1":          "P2S_1-diagnostic.jpg",
		"Workshop P2S":   "Workshop_P2S-diagnostic.jpg",
		"printer/office": "printer_office-diagnostic.jpg",
	}
	for name, want := range tests {
		if got := diagnosticSnapshotPath(name); got != want {
			t.Errorf("diagnosticSnapshotPath(%q) = %q, want %q", name, got, want)
		}
	}
}
