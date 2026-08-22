package telegram

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/sebastianrau/3d-printer-monitor/pkg/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFindChatIDsPreservesNumericChatID(t *testing.T) {
	var sentChatID string
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := `{"ok":true}`
		if strings.HasSuffix(r.URL.Path, "/getUpdates") {
			body = `{"ok":true,"result":[{"update_id":123,"message":{"text":"/id","chat":{"id":792952975,"first_name":"Sebastian","last_name":"R"}}}]}`
		} else if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request: %v", err)
			}
			form, err := url.ParseQuery(string(data))
			if err != nil {
				t.Fatalf("parse form: %v", err)
			}
			sentChatID = form.Get("chat_id")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})

	client := New(config.TelegramConfig{BotToken: "test", TimeoutSeconds: 2})
	client.HTTP = &http.Client{Transport: transport, Timeout: 2 * time.Second}
	ok, err := client.FindChatIDs(context.Background(), time.Second, true)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected lookup to succeed")
	}
	if sentChatID != "792952975" {
		t.Fatalf("chat_id = %q, want %q", sentChatID, "792952975")
	}
}

func TestSendImageUploadsInMemoryJPEG(t *testing.T) {
	want := []byte{0xff, 0xd8, 0x01, 0x02, 0xff, 0xd9}
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseMultipartForm(1024); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		file, header, err := r.FormFile("photo")
		if err != nil {
			t.Fatalf("read photo: %v", err)
		}
		defer file.Close()
		got, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("read uploaded photo: %v", err)
		}
		if string(got) != string(want) {
			t.Fatalf("uploaded bytes = %v, want %v", got, want)
		}
		if header.Filename != "printer-event.jpg" {
			t.Fatalf("filename = %q", header.Filename)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true}`)), Header: make(http.Header)}, nil
	})
	client := New(config.TelegramConfig{BotToken: "test", ChatID: "123", TimeoutSeconds: 2, Caption: "{printer}: {milestone}"})
	client.HTTP = &http.Client{Transport: transport, Timeout: 2 * time.Second}
	if err := client.SendImage(context.Background(), want, "printer-event.jpg", "P1S", "Test", 1); err != nil {
		t.Fatal(err)
	}
}
