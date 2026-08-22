package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sebastianrau/3d-printer-monitor/pkg/config"
	"github.com/sebastianrau/3d-printer-monitor/pkg/logger"
	"github.com/sebastianrau/3d-printer-monitor/pkg/messenger"
)

var log = logger.WithPackage("telegram")

var _ messenger.Messenger = (*Telegram)(nil)
var _ messenger.Service = (*Telegram)(nil)
var _ messenger.TargetDiscoverer = (*Telegram)(nil)

type Telegram struct {
	Config  config.TelegramConfig
	BaseURL string
	HTTP    *http.Client
}

func New(c config.TelegramConfig) *Telegram {
	return &Telegram{Config: c, BaseURL: "https://api.telegram.org/bot" + c.BotToken, HTTP: &http.Client{Timeout: time.Duration(c.TimeoutSeconds) * time.Second}}
}

func (t *Telegram) Validate() error {
	if t.Config.ChatID == "" {
		return fmt.Errorf("telegram is missing chat_id")
	}
	return nil
}

func (t *Telegram) Run(ctx context.Context, sources []messenger.SnapshotSource) {
	NewPoller(t, sources).Run(ctx)
}

func (t *Telegram) FindTargets(ctx context.Context, wait time.Duration, includeOld bool) (bool, error) {
	return t.FindChatIDs(ctx, wait, includeOld)
}

func (t *Telegram) SendImage(ctx context.Context, image []byte, filename, printer, milestone string, progress int) error {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fields := map[string]string{"chat_id": t.Config.ChatID, "caption": formatCaption(t.Config.Caption, printer, milestone, progress), "disable_notification": strconv.FormatBool(t.Config.DisableNotification), "protect_content": strconv.FormatBool(t.Config.ProtectContent)}
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	p, err := w.CreateFormFile("photo", filename)
	if err != nil {
		return err
	}
	if _, err = io.Copy(p, bytes.NewReader(image)); err != nil {
		return err
	}
	_ = w.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.BaseURL+"/sendPhoto", &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	return t.do(req)
}

func (t *Telegram) SendText(ctx context.Context, chatID, text string) error {
	form := url.Values{"chat_id": {chatID}, "text": {text}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.BaseURL+"/sendMessage", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return t.do(req)
}

func (t *Telegram) do(req *http.Request) error {
	r, err := t.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(b, &result); err != nil {
		return err
	}
	if r.StatusCode >= 300 || !result.OK {
		return fmt.Errorf("telegram API: status=%d %s", r.StatusCode, result.Description)
	}
	return nil
}

func (t *Telegram) FindChatIDs(ctx context.Context, wait time.Duration, includeOld bool) (bool, error) {
	offset := ""
	if !includeOld {
		updates, err := t.rawUpdates(ctx, url.Values{"offset": {"-1"}, "limit": {"1"}, "timeout": {"0"}})
		if err != nil {
			return false, err
		}
		if len(updates) > 0 {
			if id, ok := numberAsInt64(updates[len(updates)-1]["update_id"]); ok {
				offset = strconv.FormatInt(id+1, 10)
			}
		}
	}
	params := url.Values{"timeout": {strconv.Itoa(int(wait.Seconds()))}, "allowed_updates": {"[]"}}
	if offset != "" {
		params.Set("offset", offset)
	}
	log.Infof("waiting up to %d seconds for Telegram /id command", int(wait.Seconds()))
	updates, err := t.rawUpdates(ctx, params)
	if err != nil {
		return false, err
	}
	found := map[string]map[string]any{}
	for _, u := range updates {
		for _, key := range []string{"message", "edited_message", "channel_post", "edited_channel_post"} {
			m, _ := u[key].(map[string]any)
			if m == nil {
				continue
			}
			text, _ := m["text"].(string)
			fields := strings.Fields(strings.TrimSpace(text))
			if len(fields) == 0 || strings.ToLower(strings.Split(fields[0], "@")[0]) != "/id" {
				continue
			}
			chat, _ := m["chat"].(map[string]any)
			if chat == nil {
				continue
			}
			id := fmt.Sprint(chat["id"])
			found[id] = chat
		}
	}
	if len(found) == 0 {
		return false, nil
	}
	allSent := true
	for id, chat := range found {
		name := fmt.Sprint(chat["title"])
		if name == "<nil>" || name == "" {
			name = strings.TrimSpace(fmt.Sprint(chat["first_name"]) + " " + fmt.Sprint(chat["last_name"]))
		}
		log.Infof("Telegram chat found: id=%s name=%s", id, name)
		if err := t.SendText(ctx, id, "Your Telegram chat ID is: "+id); err != nil {
			log.Errorf("could not return Telegram chat ID %s: %v", id, err)
			allSent = false
		}
	}
	return allSent, nil
}

func (t *Telegram) rawUpdates(ctx context.Context, params url.Values) ([]map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.BaseURL+"/getUpdates?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	client := t.HTTP
	if seconds, err := strconv.Atoi(params.Get("timeout")); err == nil && seconds > 0 {
		client = &http.Client{Timeout: time.Duration(seconds+10) * time.Second, Transport: t.HTTP.Transport}
	}
	r, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	var out struct {
		OK          bool             `json:"ok"`
		Result      []map[string]any `json:"result"`
		Description string           `json:"description"`
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber()
	if err := decoder.Decode(&out); err != nil {
		return nil, err
	}
	if !out.OK {
		return nil, fmt.Errorf("telegram getUpdates: %s", out.Description)
	}
	return out.Result, nil
}

func numberAsInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case json.Number:
		x, e := n.Int64()
		return x, e == nil
	}
	return 0, false
}

func formatCaption(t, p, m string, n int) string {
	r := strings.NewReplacer("{printer}", p, "{milestone}", m, "{progress}", strconv.Itoa(n))
	return r.Replace(t)
}
