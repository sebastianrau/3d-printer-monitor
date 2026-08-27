package telegram

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sebastianrau/3d-printer-monitor/pkg/config"
	"github.com/sebastianrau/3d-printer-monitor/pkg/messenger"
)

func TestProgressBarHasTenSegments(t *testing.T) {
	got := progressBar(57)
	if strings.Count(got, "🟩") != 5 || strings.Count(got, "⬜") != 5 {
		t.Fatalf("progressBar(57) = %q", got)
	}
}

func TestFormatPrintStatusIncludesAvailableDetails(t *testing.T) {
	progress, layer, total, remaining := 57, 144, 252, 74
	nozzle, bed := 260.0, 100.0
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.Local)
	text := formatPrintStatus(messenger.PrintStatus{Printer: "P1S", Job: "Gehäusedeckel", State: "RUNNING", Stage: "Cleaning nozzle tip", Progress: &progress, Layer: &layer, TotalLayers: &total, RemainingMinutes: &remaining, NozzleTemperature: &nozzle, BedTemperature: &bed}, now)
	for _, want := range []string{"🖨️ P1S – Gehäusedeckel", "57 %", "🔄 Status: Drucken", "🛠️ Vorgang: Cleaning nozzle tip", "📚 Layer: 144 / 252", "⏳ Restzeit: 1 h 14 min", "🏁 Fertig um: 13:14 Uhr", "🔥 Düse: 260 °C", "🌡️ Bett: 100 °C"} {
		if !strings.Contains(text, want) {
			t.Errorf("status missing %q:\n%s", want, text)
		}
	}
}

func TestStatusUpdateRequiresChangedContentAndInterval(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	var calls []string
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls = append(calls, request.URL.Path)
		body := `{"ok":true,"result":{"message_id":42}}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	client := New(config.TelegramConfig{BotToken: "test", ChatID: "123", TimeoutSeconds: 2})
	client.HTTP = &http.Client{Transport: transport}
	client.now = func() time.Time { return now }
	progress := 1
	status := messenger.PrintStatus{Printer: "P2S", Job: "Part", State: "RUNNING", Progress: &progress}
	if err := client.PublishPrintStatus(context.Background(), "printer:job", status, true, false); err != nil {
		t.Fatal(err)
	}
	if err := client.PublishPrintStatus(context.Background(), "printer:job", status, false, false); err != nil {
		t.Fatal(err)
	}
	progress = 2
	status.Progress = &progress
	if err := client.PublishPrintStatus(context.Background(), "printer:job", status, false, false); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 {
		t.Fatalf("calls before interval = %d, want 1", len(calls))
	}
	now = now.Add(statusUpdateInterval)
	if err := client.PublishPrintStatus(context.Background(), "printer:job", status, false, false); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || !strings.HasSuffix(calls[1], "/editMessageText") {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestImmediateStatusChangeBypassesInterval(t *testing.T) {
	var calls int
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":{"message_id":42}}`)), Header: make(http.Header)}, nil
	})
	client := New(config.TelegramConfig{BotToken: "test", ChatID: "123", TimeoutSeconds: 2})
	client.HTTP = &http.Client{Transport: transport}
	progress := 10
	running := messenger.PrintStatus{Printer: "P2S", State: "RUNNING", Progress: &progress}
	if err := client.PublishPrintStatus(context.Background(), "key", running, true, false); err != nil {
		t.Fatal(err)
	}
	running.State = "PAUSE"
	if err := client.PublishPrintStatus(context.Background(), "key", running, true, false); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestTelegram429UsesRetryAfter(t *testing.T) {
	requests := 0
	var waited time.Duration
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		body, status := `{"ok":true}`, http.StatusOK
		if requests == 1 {
			body, status = `{"ok":false,"parameters":{"retry_after":3}}`, http.StatusTooManyRequests
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	client := New(config.TelegramConfig{BotToken: "test", ChatID: "123", TimeoutSeconds: 2})
	client.HTTP = &http.Client{Transport: transport}
	client.wait = func(_ context.Context, duration time.Duration) error { waited = duration; return nil }
	if err := client.SendText(context.Background(), "123", "test"); err != nil {
		t.Fatal(err)
	}
	if requests != 2 || waited != 3*time.Second {
		t.Fatalf("requests=%d waited=%s", requests, waited)
	}
}
