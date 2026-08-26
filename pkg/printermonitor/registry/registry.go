// Package registry maps configured model names to printer implementations.
package registry

import (
	"fmt"
	"strings"

	"github.com/sebastianrau/3d-printer-monitor/pkg/config"
	"github.com/sebastianrau/3d-printer-monitor/pkg/printermonitor"
	"github.com/sebastianrau/3d-printer-monitor/pkg/printermonitor/bambu/p1s"
	"github.com/sebastianrau/3d-printer-monitor/pkg/printermonitor/bambu/p2s"
)

type Factory func(config.Printer) (printermonitor.Printer, error)

type Registry struct{ factories map[string]Factory }

func New() *Registry {
	r := &Registry{factories: map[string]Factory{}}
	// These aliases retain the existing behavior. Dedicated implementations
	// can replace them later without changing the shared monitor.
	for _, model := range []string{"p1", "p1s"} {
		r.Register("bambu/"+model, func(c config.Printer) (printermonitor.Printer, error) {
			return p1s.New(c), nil
		})
	}
	r.Register("bambu/p2s", func(c config.Printer) (printermonitor.Printer, error) {
		return p2s.New(c), nil
	})
	return r
}

func (r *Registry) Register(key string, factory Factory) {
	r.factories[strings.ToLower(strings.TrimSpace(key))] = factory
}

func (r *Registry) Create(c config.Printer) (printermonitor.Printer, error) {
	key := strings.ToLower(strings.TrimSpace(c.RegistryKey()))
	factory, ok := r.factories[key]
	if !ok {
		return nil, fmt.Errorf("unsupported printer implementation %q", key)
	}
	return factory(c)
}
