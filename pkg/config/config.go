package config

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	LogLevel  string    `yaml:"log_level"`
	Printers  []Printer `yaml:"printers"`
	Messaging Messaging `yaml:"messaging"`
}

type Messaging struct {
	Provider string         `yaml:"provider"`
	Telegram TelegramConfig `yaml:"telegram"`
}

type TelegramConfig struct {
	BotToken                  string  `yaml:"bot_token"`
	ChatID                    string  `yaml:"chat_id"`
	Caption                   string  `yaml:"caption"`
	DisableNotification       bool    `yaml:"disable_notification"`
	ProtectContent            bool    `yaml:"protect_content"`
	TimeoutSeconds            int     `yaml:"timeout_seconds"`
	CommandsEnabled           *bool   `yaml:"commands_enabled"`
	CommandPollTimeoutSeconds int     `yaml:"command_poll_timeout_seconds"`
	CommandCooldownSeconds    float64 `yaml:"command_cooldown_seconds"`
}

type Printer struct {
	Name                 string          `yaml:"name"`
	ID                   string          `yaml:"id"`
	Type                 string          `yaml:"type"`
	Enabled              *bool           `yaml:"enabled"`
	Bambu                *BambuPrinter   `yaml:"bambu"`
	EventQueueSize       int             `yaml:"event_queue_size"`
	DeliveryAttempts     int             `yaml:"delivery_attempts"`
	DeliveryRetrySeconds float64         `yaml:"delivery_retry_seconds"`
	Notifications        map[string]bool `yaml:"notifications"`
}

type BambuPrinter struct {
	Model                   string  `yaml:"model"`
	Host                    string  `yaml:"host"`
	Serial                  string  `yaml:"serial"`
	AccessCode              string  `yaml:"access_code"`
	CameraTimeoutSeconds    float64 `yaml:"camera_timeout_seconds"`
	CameraWarmupFrames      *int    `yaml:"camera_warmup_frames"`
	MQTTReconnectMinSeconds int     `yaml:"mqtt_reconnect_min_seconds"`
	MQTTReconnectMaxSeconds int     `yaml:"mqtt_reconnect_max_seconds"`
}

func (p Printer) IsEnabled() bool { return p.Enabled == nil || *p.Enabled }
func (p Printer) DisplayName() string {
	if p.Name != "" {
		return p.Name
	}
	return p.Identifier()
}
func (p Printer) Notify(key string) bool { v, ok := p.Notifications[key]; return !ok || v }
func (p Printer) Identifier() string {
	if p.ID != "" {
		return p.ID
	}
	if p.Bambu != nil {
		return p.Bambu.Serial
	}
	return ""
}
func (p Printer) RegistryKey() string {
	if p.Type == "bambu" && p.Bambu != nil {
		return "bambu/" + p.Bambu.Model
	}
	return p.Type
}
func (p Printer) BambuSettings() BambuPrinter {
	if p.Bambu != nil {
		return *p.Bambu
	}
	return BambuPrinter{}
}

func (p BambuPrinter) WarmupFrames() int {
	if p.CameraWarmupFrames == nil {
		return 2
	}
	return *p.CameraWarmupFrames
}

func Load(path string, requireMessenger, requireTarget bool) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var c Config
	decoder := yaml.NewDecoder(bytes.NewReader(b))
	decoder.KnownFields(true)
	if err := decoder.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	c.defaults()
	if err := c.Validate(requireMessenger, requireTarget); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (c *Config) defaults() {
	if c.LogLevel == "" {
		c.LogLevel = "INFO"
	}
	if c.Messaging.Provider == "" {
		c.Messaging.Provider = "telegram"
	}
	if c.Messaging.Telegram.Caption == "" {
		c.Messaging.Telegram.Caption = "🖨️ {printer}: {milestone} ({progress}%)"
	}
	if c.Messaging.Telegram.TimeoutSeconds == 0 {
		c.Messaging.Telegram.TimeoutSeconds = 60
	}
	if c.Messaging.Telegram.CommandPollTimeoutSeconds == 0 {
		c.Messaging.Telegram.CommandPollTimeoutSeconds = 60
	}
	if c.Messaging.Telegram.CommandCooldownSeconds == 0 {
		c.Messaging.Telegram.CommandCooldownSeconds = 10
	}
	for i := range c.Printers {
		p := &c.Printers[i]
		p.Type = strings.ToLower(strings.TrimSpace(p.Type))
		if p.Type == "" {
			p.Type = "bambu"
		}
		bambu := p.BambuSettings()
		bambu.Model = strings.ToLower(strings.TrimSpace(bambu.Model))
		if bambu.Model == "" {
			bambu.Model = "p1s"
		}
		if bambu.CameraTimeoutSeconds == 0 {
			bambu.CameraTimeoutSeconds = 10
		}
		if bambu.MQTTReconnectMinSeconds == 0 {
			bambu.MQTTReconnectMinSeconds = 2
		}
		if bambu.MQTTReconnectMaxSeconds == 0 {
			bambu.MQTTReconnectMaxSeconds = 60
		}
		if p.Type == "bambu" {
			p.Bambu = &bambu
		}
		if p.EventQueueSize == 0 {
			p.EventQueueSize = 16
		}
		if p.DeliveryAttempts == 0 {
			p.DeliveryAttempts = 3
		}
		if p.DeliveryRetrySeconds == 0 {
			p.DeliveryRetrySeconds = 5
		}
	}
}

func (c Config) Validate(requireMessenger, requireTarget bool) error {
	if len(c.Printers) == 0 {
		return fmt.Errorf("at least one printer must be configured")
	}
	enabled := 0
	bambuModels := map[string]bool{"p1": true, "p1s": true, "p2s": true}
	for i, p := range c.Printers {
		if !p.IsEnabled() {
			continue
		}
		enabled++
		if p.Type != "bambu" {
			return fmt.Errorf("enabled printer %d has unsupported type %q", i+1, p.Type)
		}
		bambu := p.BambuSettings()
		if bambu.Host == "" || bambu.Serial == "" || bambu.AccessCode == "" {
			return fmt.Errorf("enabled Bambu printer %d requires host, serial and access_code", i+1)
		}
		if !bambuModels[bambu.Model] {
			return fmt.Errorf("enabled Bambu printer %d has unsupported model %q", i+1, bambu.Model)
		}
		if p.EventQueueSize < 1 || p.DeliveryAttempts < 1 || bambu.WarmupFrames() < 0 {
			return fmt.Errorf("invalid numeric setting for printer %d", i+1)
		}
	}
	if enabled == 0 {
		return fmt.Errorf("at least one printer must be enabled")
	}
	_ = requireMessenger
	_ = requireTarget
	return nil
}
