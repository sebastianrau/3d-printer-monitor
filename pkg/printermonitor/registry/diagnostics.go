package registry

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/sebastianrau/3d-printer-monitor/pkg/config"
	"github.com/sebastianrau/3d-printer-monitor/pkg/logger"
)

var log = logger.WithPackage("printer-registry")

// Diagnose runs the selected printer implementation's diagnostic operation.
// Transport-specific checks remain inside that implementation.
func (r *Registry) Diagnose(ctx context.Context, printers []config.Printer, printerName string, timeout time.Duration) error {
	for _, pc := range printers {
		if !pc.IsEnabled() || !strings.EqualFold(pc.DisplayName(), printerName) {
			continue
		}
		implementation, err := r.Create(pc)
		if err != nil {
			return fmt.Errorf("create printer %q: %w", pc.DisplayName(), err)
		}
		image, err := implementation.Diagnose(ctx, timeout)
		if err != nil {
			return fmt.Errorf("diagnose printer %q: %w", pc.DisplayName(), err)
		}
		snapshotPath := diagnosticSnapshotPath(pc.DisplayName())
		if err := os.WriteFile(snapshotPath, image, 0600); err != nil {
			return fmt.Errorf("store diagnostic snapshot: %w", err)
		}
		log.Infof("[%s] printer diagnostic passed; snapshot stored at %s", pc.DisplayName(), snapshotPath)
		return nil
	}
	return fmt.Errorf("enabled printer %q not found", printerName)
}

func diagnosticSnapshotPath(printerName string) string {
	name := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, strings.TrimSpace(printerName))
	if name == "" {
		name = "printer"
	}
	return name + "-diagnostic.jpg"
}
