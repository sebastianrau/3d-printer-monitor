package registry

import (
	"fmt"
	"strings"

	"github.com/sebastianrau/3d-printer-monitor/pkg/config"
	"github.com/sebastianrau/3d-printer-monitor/pkg/messenger"
	"github.com/sebastianrau/3d-printer-monitor/pkg/messenger/telegram"
)

type Factory func(config.Messaging) (messenger.Service, error)

type Registry struct {
	factories map[string]Factory
}

func New() *Registry {
	r := &Registry{factories: make(map[string]Factory)}
	r.Register("telegram", newTelegram)
	return r
}

func (r *Registry) Register(provider string, factory Factory) {
	r.factories[strings.ToLower(strings.TrimSpace(provider))] = factory
}

func (r *Registry) Create(c config.Messaging) (messenger.Service, error) {
	provider := strings.ToLower(strings.TrimSpace(c.Provider))
	factory, ok := r.factories[provider]
	if !ok {
		return nil, fmt.Errorf("unsupported messaging provider %q", provider)
	}
	return factory(c)
}

func newTelegram(c config.Messaging) (messenger.Service, error) {
	t := c.Telegram
	if t.BotToken == "" {
		return nil, fmt.Errorf("telegram is missing bot_token")
	}
	return telegram.New(t), nil
}
