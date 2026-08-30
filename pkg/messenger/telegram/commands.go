// Package telegram implements the Messenger abstraction and bot commands.
package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sebastianrau/3d-printer-monitor/pkg/messenger"
)

type Poller struct {
	Telegram *Telegram
	Monitors []messenger.SnapshotSource
	Offset   int64
	last     map[string]time.Time
}
type update struct {
	UpdateID    int64    `json:"update_id"`
	Message     *message `json:"message"`
	ChannelPost *message `json:"channel_post"`
}
type message struct {
	Text string `json:"text"`
	Chat struct {
		ID json.Number `json:"id"`
	} `json:"chat"`
}

func NewPoller(t *Telegram, monitors []messenger.SnapshotSource) *Poller {
	return &Poller{Telegram: t, Monitors: monitors, last: map[string]time.Time{}}
}
func (p *Poller) Run(ctx context.Context) {
	if p.Telegram.Config.CommandsEnabled != nil && !*p.Telegram.Config.CommandsEnabled {
		return
	}
	_ = p.discard(ctx)
	for ctx.Err() == nil {
		updates, err := p.get(ctx, p.Telegram.Config.CommandPollTimeoutSeconds)
		if err != nil {
			if ctx.Err() == nil {
				log.Errorf("Telegram polling failed: %v", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
				}
			}
			continue
		}
		for _, u := range updates {
			p.Offset = u.UpdateID + 1
			p.handle(ctx, u)
		}
	}
}
func (p *Poller) discard(ctx context.Context) error {
	u, e := p.getWith(ctx, url.Values{"offset": {"-1"}, "limit": {"1"}, "timeout": {"0"}})
	if len(u) > 0 {
		p.Offset = u[len(u)-1].UpdateID + 1
	}
	return e
}
func (p *Poller) get(ctx context.Context, timeout int) ([]update, error) {
	v := url.Values{"timeout": {strconv.Itoa(timeout)}, "allowed_updates": {`["message","channel_post"]`}}
	if p.Offset > 0 {
		v.Set("offset", strconv.FormatInt(p.Offset, 10))
	}
	return p.getWith(ctx, v)
}
func (p *Poller) getWith(ctx context.Context, v url.Values) ([]update, error) {
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, p.Telegram.BaseURL+"/getUpdates?"+v.Encode(), nil)
	if e != nil {
		return nil, e
	}
	r, e := p.Telegram.longPollClient(v).Do(req)
	if e != nil {
		return nil, e
	}
	defer func() { _ = r.Body.Close() }()
	b, _ := io.ReadAll(r.Body)
	var out struct {
		OK          bool     `json:"ok"`
		Result      []update `json:"result"`
		Description string   `json:"description"`
	}
	if e = json.Unmarshal(b, &out); e != nil {
		return nil, e
	}
	if !out.OK {
		return nil, fmt.Errorf("telegram: %s", out.Description)
	}
	return out.Result, nil
}
func (p *Poller) handle(ctx context.Context, u update) {
	m := u.Message
	if m == nil {
		m = u.ChannelPost
	}
	if m == nil {
		return
	}
	parts := strings.Fields(m.Text)
	if len(parts) == 0 {
		return
	}
	cmd := strings.ToLower(strings.Split(parts[0], "@")[0])
	if cmd != "/snapshot" && cmd != "/snapshop" {
		return
	}
	chat := m.Chat.ID.String()
	if chat != p.Telegram.Config.ChatID {
		log.Warnf("ignoring unauthorized Telegram command from chat %s", chat)
		return
	}
	if since := time.Since(p.last[chat]); !p.last[chat].IsZero() && since < time.Duration(p.Telegram.Config.CommandCooldownSeconds*float64(time.Second)) {
		_ = p.Telegram.SendText(ctx, chat, "Bitte kurz bis zum nächsten Snapshot warten.")
		return
	}
	selector := ""
	if len(parts) > 1 {
		selector = strings.Join(parts[1:], " ")
	}
	targets := p.selectMonitors(selector)
	if len(targets) == 0 {
		_ = p.Telegram.SendText(ctx, chat, "Drucker nicht gefunden.")
		return
	}
	if selector != "" && len(targets) > 1 {
		_ = p.Telegram.SendText(ctx, chat, "Auswahl ist nicht eindeutig.")
		return
	}
	queued := 0
	for _, r := range targets {
		if r.RequestManualSnapshot() {
			queued++
		}
	}
	if queued == 0 {
		_ = p.Telegram.SendText(ctx, chat, "Snapshot konnte nicht eingereiht werden.")
		return
	}
	p.last[chat] = time.Now()
}
func (p *Poller) selectMonitors(s string) []messenger.SnapshotSource {
	if s == "" {
		return p.Monitors
	}
	wanted := strings.ToLower(s)
	var exact, out []messenger.SnapshotSource
	for _, r := range p.Monitors {
		n, identifier := strings.ToLower(r.Name()), strings.ToLower(r.Identifier())
		if n == wanted || identifier == wanted {
			exact = append(exact, r)
		} else if strings.Contains(n, wanted) || strings.HasPrefix(identifier, wanted) {
			out = append(out, r)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return out
}
