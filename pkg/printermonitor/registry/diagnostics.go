package registry

import (
	"context"
	"fmt"
	"strings"
	"time"

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
		err = implementation.Diagnose(ctx, timeout)
		if err != nil {
			return fmt.Errorf("diagnose printer %q: %w", pc.DisplayName(), err)
		}
		log.Infof("[%s] printer diagnostic passed", pc.DisplayName())
		return nil
	}
	return fmt.Errorf("enabled printer %q not found", printerName)
}
