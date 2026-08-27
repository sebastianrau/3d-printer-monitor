package telegram

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sebastianrau/3d-printer-monitor/pkg/messenger"
)

const statusUpdateInterval = 60 * time.Second

type statusMessage struct {
	messageID int64
	text      string
	updatedAt time.Time
}

func (t *Telegram) PublishPrintStatus(ctx context.Context, key string, status messenger.PrintStatus, immediate, terminal bool) error {
	t.statusMu.Lock()
	defer t.statusMu.Unlock()
	now := t.now()
	text := formatPrintStatus(status, now)
	previous, exists := t.statuses[key]
	if exists && text == previous.text {
		if terminal {
			delete(t.statuses, key)
		}
		return nil
	}
	if exists && !immediate && now.Sub(previous.updatedAt) < statusUpdateInterval {
		return nil
	}
	if !exists {
		id, err := t.sendStatus(ctx, text)
		if err != nil {
			return err
		}
		t.statuses[key] = statusMessage{messageID: id, text: text, updatedAt: now}
	} else {
		if err := t.editStatus(ctx, previous.messageID, text); err != nil {
			return err
		}
		t.statuses[key] = statusMessage{messageID: previous.messageID, text: text, updatedAt: now}
	}
	if terminal {
		delete(t.statuses, key)
	}
	return nil
}

func (t *Telegram) sendStatus(ctx context.Context, text string) (int64, error) {
	form := url.Values{"chat_id": {t.Config.ChatID}, "text": {text}, "disable_notification": {strconv.FormatBool(t.Config.DisableNotification)}, "protect_content": {strconv.FormatBool(t.Config.ProtectContent)}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.BaseURL+"/sendMessage", strings.NewReader(form.Encode()))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var result struct {
		MessageID int64 `json:"message_id"`
	}
	if err := t.doInto(req, &result); err != nil {
		return 0, err
	}
	return result.MessageID, nil
}

func (t *Telegram) editStatus(ctx context.Context, messageID int64, text string) error {
	form := url.Values{"chat_id": {t.Config.ChatID}, "message_id": {strconv.FormatInt(messageID, 10)}, "text": {text}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.BaseURL+"/editMessageText", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return t.do(req)
}

func progressBar(progress int) string {
	progress = max(0, min(100, progress))
	complete := int(math.Floor(float64(progress) * 20 / 100))
	return strings.Repeat("🟩", complete) + strings.Repeat("⬜", 20-complete)
}

func formatPrintStatus(s messenger.PrintStatus, now time.Time) string {
	progress := 0
	if s.Progress != nil {
		progress = *s.Progress
	}
	job := s.Job
	if job == "" {
		job = "Druckauftrag"
	}
	lines := []string{fmt.Sprintf("🖨️ %s – %s", s.Printer, job), fmt.Sprintf("%s %d %%", progressBar(progress), progress), "", fmt.Sprintf("%s Status: %s", statusEmoji(s.State), statusLabel(s.State))}
	if s.Stage != "" {
		lines = append(lines, "🛠️ Vorgang: "+s.Stage)
	}
	if s.Layer != nil {
		layer := fmt.Sprintf("📚 Layer: %d", *s.Layer)
		if s.TotalLayers != nil {
			layer += fmt.Sprintf(" / %d", *s.TotalLayers)
		}
		lines = append(lines, layer)
	}
	if s.RemainingMinutes != nil {
		lines = append(lines, "⏳ Restzeit: "+formatMinutes(*s.RemainingMinutes))
		finishedAt := now.Add(time.Duration(*s.RemainingMinutes) * time.Minute)
		lines = append(lines, "🏁 Fertig um: "+finishedAt.Format("15:04")+" Uhr")
	}
	if s.NozzleTemperature != nil {
		lines = append(lines, fmt.Sprintf("🔥 Düse: %.0f °C", *s.NozzleTemperature))
	}
	if s.BedTemperature != nil {
		lines = append(lines, fmt.Sprintf("🌡️ Bett: %.0f °C", *s.BedTemperature))
	}
	return strings.Join(lines, "\n")
}

func statusEmoji(state string) string {
	switch strings.ToUpper(state) {
	case "PREPARE", "STARTED":
		return "▶️"
	case "RUNNING":
		return "🔄"
	case "PAUSE", "PAUSED":
		return "⏸️"
	case "FINISH", "FINISHED":
		return "✅"
	case "FAILED", "ABORTED":
		return "⛔"
	case "WARNING":
		return "⚠️"
	case "ERROR":
		return "❌"
	case "OFFLINE":
		return "📡"
	case "ONLINE":
		return "🟢"
	default:
		return "ℹ️"
	}
}
func statusLabel(state string) string {
	switch strings.ToUpper(state) {
	case "PREPARE", "STARTED":
		return "Druck gestartet"
	case "RUNNING":
		return "Drucken"
	case "PAUSE", "PAUSED":
		return "Pausiert"
	case "FINISH", "FINISHED":
		return "Abgeschlossen"
	case "FAILED", "ABORTED":
		return "Abgebrochen"
	case "WARNING":
		return "Warnung"
	case "ERROR":
		return "Fehler"
	case "OFFLINE":
		return "Drucker offline"
	case "ONLINE":
		return "Drucker online"
	default:
		return state
	}
}
func formatMinutes(minutes int) string {
	if minutes < 60 {
		return fmt.Sprintf("%d min", minutes)
	}
	return fmt.Sprintf("%d h %d min", minutes/60, minutes%60)
}
