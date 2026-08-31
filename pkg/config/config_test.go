package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNestedBambuConfiguration(t *testing.T) {
	c := loadTestConfig(t, `
printers:
  - name: Office
    type: bambu
    bambu:
      model: p1s
      host: 192.0.2.1
      serial: SERIAL
      access_code: CODE
`)
	p := c.Printers[0]
	if p.RegistryKey() != "bambu/p1s" || p.Identifier() != "SERIAL" {
		t.Fatalf("printer normalization failed: %#v", p)
	}
	if p.Bambu == nil || p.Bambu.Host != "192.0.2.1" {
		t.Fatalf("Bambu settings missing: %#v", p.Bambu)
	}
}

func TestTelegramTimeoutDefaults(t *testing.T) {
	c := loadTestConfig(t, `
printers:
  - type: bambu
    bambu:
      host: 192.0.2.1
      serial: SERIAL
      access_code: CODE
`)
	if got := c.Messaging.Telegram.TimeoutSeconds; got != 60 {
		t.Fatalf("Telegram HTTP timeout = %d, want 60", got)
	}
	if got := c.Messaging.Telegram.CommandPollTimeoutSeconds; got != 60 {
		t.Fatalf("command poll timeout = %d, want 60", got)
	}
}

func TestFlatBambuConfigurationIsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("printers:\n  - name: Office\n    model: p1s\n    host: 192.0.2.1\n    serial: SERIAL\n    access_code: CODE\n")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, false, false); err == nil {
		t.Fatal("expected flat Bambu configuration to be rejected")
	}
}

func TestExampleConfigurationContainsOnlySupportedOptions(t *testing.T) {
	if _, err := Load(filepath.Join("..", "..", "config.example.yaml"), true, true); err != nil {
		t.Fatal(err)
	}
}

func TestUnsupportedBambuModelsAreRejected(t *testing.T) {
	for _, model := range []string{"x1", "x1c"} {
		t.Run(model, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			content := []byte("printers:\n  - type: bambu\n    bambu:\n      model: " + model + "\n      host: 192.0.2.1\n      serial: SERIAL\n      access_code: CODE\n")
			if err := os.WriteFile(path, content, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path, false, false); err == nil {
				t.Fatalf("expected model %q to be rejected", model)
			}
		})
	}
}

func TestP2SConfigurationIsSupported(t *testing.T) {
	c := loadTestConfig(t, `
printers:
  - type: bambu
    bambu:
      model: p2s
      host: 192.0.2.1
      serial: P2SERIAL
      access_code: CODE
`)
	if got := c.Printers[0].RegistryKey(); got != "bambu/p2s" {
		t.Fatalf("registry key = %q", got)
	}
}

func loadTestConfig(t *testing.T, content string) Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path, false, false)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
