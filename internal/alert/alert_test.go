package alert

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

func TestHTTPProviderDiscord(t *testing.T) {
	var received map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := GetProvider(models.AlertConfig{Type: "discord", Settings: map[string]string{"url": srv.URL}})
	if err := p.Send(context.Background(), "Test Title", "Test Body"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if received["content"] != "**Test Title**\nTest Body" {
		t.Errorf("unexpected payload: %s", received["content"])
	}
}

func TestHTTPProviderSlack(t *testing.T) {
	var received map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := GetProvider(models.AlertConfig{Type: "slack", Settings: map[string]string{"url": srv.URL}})
	if err := p.Send(context.Background(), "Alert", "Message"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if received["text"] != "*Alert*\nMessage" {
		t.Errorf("unexpected payload: %s", received["text"])
	}
}

func TestHTTPProviderWebhook(t *testing.T) {
	var received map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := GetProvider(models.AlertConfig{Type: "webhook", Settings: map[string]string{"url": srv.URL}})
	if err := p.Send(context.Background(), "Title", "Body"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if received["title"] != "Title" || received["message"] != "Body" || received["status"] != "alert" {
		t.Errorf("unexpected webhook payload: %v", received)
	}
}

func TestHTTPProviderErrorOnHTTP4xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
	}))
	defer srv.Close()

	p := GetProvider(models.AlertConfig{Type: "discord", Settings: map[string]string{"url": srv.URL}})
	if err := p.Send(context.Background(), "Test", "Test"); err == nil {
		t.Fatal("expected error on 403 response")
	}
}

func TestNtfyProvider(t *testing.T) {
	var title, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		title = r.Header.Get("Title")
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		body = string(buf[:n])
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := GetProvider(models.AlertConfig{Type: "ntfy", Settings: map[string]string{
		"url":   srv.URL,
		"topic": "test",
	}})
	if err := p.Send(context.Background(), "Alert Title", "Alert Body"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if title != "Alert Title" {
		t.Errorf("expected title 'Alert Title', got '%s'", title)
	}
	if body != "Alert Body" {
		t.Errorf("expected body 'Alert Body', got '%s'", body)
	}
}

func TestHTTPProviderTelegram(t *testing.T) {
	var received map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := &HTTPProvider{URL: srv.URL, Payload: telegramPayload("12345")}
	if err := p.Send(context.Background(), "Alert", "Down"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if received["chat_id"] != "12345" {
		t.Errorf("expected chat_id '12345', got '%s'", received["chat_id"])
	}
	if received["text"] != "*Alert*\nDown" {
		t.Errorf("unexpected text: %s", received["text"])
	}
	if received["parse_mode"] != "Markdown" {
		t.Errorf("expected parse_mode 'Markdown', got '%s'", received["parse_mode"])
	}
}

func TestHTTPProviderPagerDuty(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := &HTTPProvider{URL: srv.URL, Payload: pagerdutyPayload("test-key", "critical")}
	if err := p.Send(context.Background(), "Alert", "Down"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if received["routing_key"] != "test-key" {
		t.Errorf("expected routing_key 'test-key', got '%v'", received["routing_key"])
	}
	if received["event_action"] != "trigger" {
		t.Errorf("expected event_action 'trigger', got '%v'", received["event_action"])
	}
	payload := received["payload"].(map[string]any)
	if payload["summary"] != "Alert: Down" {
		t.Errorf("unexpected summary: %v", payload["summary"])
	}
	if payload["severity"] != "critical" {
		t.Errorf("expected severity 'critical', got '%v'", payload["severity"])
	}
}

func TestHTTPProviderPushover(t *testing.T) {
	var received map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := &HTTPProvider{URL: srv.URL, Payload: pushoverPayload("app-tok", "user-key")}
	if err := p.Send(context.Background(), "Alert", "Down"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if received["token"] != "app-tok" {
		t.Errorf("expected token 'app-tok', got '%s'", received["token"])
	}
	if received["user"] != "user-key" {
		t.Errorf("expected user 'user-key', got '%s'", received["user"])
	}
	if received["title"] != "Alert" || received["message"] != "Down" {
		t.Errorf("unexpected payload: %v", received)
	}
}

func TestHTTPProviderGotify(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := &HTTPProvider{URL: srv.URL, Payload: gotifyPayload("8")}
	if err := p.Send(context.Background(), "Alert", "Down"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if received["title"] != "Alert" || received["message"] != "Down" {
		t.Errorf("unexpected payload: %v", received)
	}
	if pri, ok := received["priority"].(float64); !ok || pri != 8 {
		t.Errorf("expected priority 8, got %v", received["priority"])
	}
}

func TestHTTPProviderOpsgenie(t *testing.T) {
	var received map[string]any
	var authHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(202)
	}))
	defer srv.Close()

	p := GetProvider(models.AlertConfig{Type: "opsgenie", Settings: map[string]string{
		"api_key":  "test-genie-key",
		"priority": "P1",
	}})
	hp := p.(*HTTPProvider)
	hp.URL = srv.URL

	if err := p.Send(context.Background(), "Site Down", "mysite.com is unreachable"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if authHeader != "GenieKey test-genie-key" {
		t.Errorf("expected auth 'GenieKey test-genie-key', got '%s'", authHeader)
	}
	if received["message"] != "Site Down" {
		t.Errorf("unexpected message: %v", received["message"])
	}
	if received["description"] != "mysite.com is unreachable" {
		t.Errorf("unexpected description: %v", received["description"])
	}
	if received["source"] != "uptop" {
		t.Errorf("expected source 'uptop', got '%v'", received["source"])
	}
	if received["priority"] != "P1" {
		t.Errorf("expected priority 'P1', got '%v'", received["priority"])
	}
}

func TestOpsgenieEUEndpoint(t *testing.T) {
	p := GetProvider(models.AlertConfig{Type: "opsgenie", Settings: map[string]string{
		"api_key": "key", "eu": "true",
	}})
	hp := p.(*HTTPProvider)
	if hp.URL != "https://api.eu.opsgenie.com/v2/alerts" {
		t.Errorf("expected EU URL, got '%s'", hp.URL)
	}
}

func TestOpsgenieUSEndpoint(t *testing.T) {
	p := GetProvider(models.AlertConfig{Type: "opsgenie", Settings: map[string]string{
		"api_key": "key",
	}})
	hp := p.(*HTTPProvider)
	if hp.URL != "https://api.opsgenie.com/v2/alerts" {
		t.Errorf("expected US URL, got '%s'", hp.URL)
	}
}

func TestLimitMessage(t *testing.T) {
	short := "short"
	if got := limitMessage(short, 130); got != short {
		t.Errorf("expected '%s', got '%s'", short, got)
	}
	long := string(make([]byte, 200))
	if got := limitMessage(long, 130); len(got) != 130 {
		t.Errorf("expected length 130, got %d", len(got))
	}
}

func TestGetProviderNewTypes(t *testing.T) {
	for _, typ := range []string{"telegram", "pagerduty", "pushover", "gotify", "opsgenie"} {
		p := GetProvider(models.AlertConfig{Type: typ, Settings: map[string]string{
			"token": "x", "chat_id": "1", "routing_key": "k", "user": "u", "url": "http://localhost",
		}})
		if p == nil {
			t.Errorf("GetProvider(%q) returned nil", typ)
		}
	}
}

func TestGetProviderUnknown(t *testing.T) {
	p := GetProvider(models.AlertConfig{Type: "unknown"})
	if p != nil {
		t.Error("expected nil for unknown provider type")
	}
}

func TestSanitizeHeader(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"normal subject", "normal subject"},
		{"inject\r\nBcc: evil@bad.com", "injectBcc: evil@bad.com"},
		{"has\nnewline", "hasnewline"},
		{"has\rcarriage", "hascarriage"},
	}
	for _, tt := range tests {
		got := sanitizeHeader(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeHeader(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// sanitizeError must strip the credential-bearing URL from a *url.Error while
// keeping the operation and underlying cause.
func TestSanitizeError(t *testing.T) {
	urlErr := &url.Error{
		Op:  "Post",
		URL: "https://api.telegram.org/bot123456:SECRET_TOKEN/sendMessage",
		Err: errors.New("dial tcp: connection refused"),
	}
	got := sanitizeError(urlErr).Error()

	for _, leak := range []string{"SECRET_TOKEN", "api.telegram.org", "sendMessage", "bot123456"} {
		if strings.Contains(got, leak) {
			t.Errorf("sanitized error leaked %q: %s", leak, got)
		}
	}
	if !strings.Contains(got, "connection refused") {
		t.Errorf("expected underlying cause preserved, got: %s", got)
	}

	// Non-url errors pass through unchanged.
	plain := errors.New("plain failure")
	if sanitizeError(plain).Error() != "plain failure" {
		t.Errorf("non-url error altered: %s", sanitizeError(plain))
	}
	if sanitizeError(nil) != nil {
		t.Error("nil should stay nil")
	}
}

func TestEmailProvider_ContextTimeout(t *testing.T) {
	// Listener that accepts but never speaks — simulates a blackholed SMTP server.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// Hold connection open, never send banner.
			go func(c net.Conn) {
				time.Sleep(30 * time.Second)
				c.Close()
			}(conn)
		}
	}()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	provider := &EmailProvider{
		Host: "127.0.0.1", Port: portStr,
		From: "test@test.com", To: "dest@test.com",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	err = provider.Send(ctx, "test", "body")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from stalled SMTP")
	}
	if elapsed > 2*time.Second {
		t.Errorf("Send took %v — context deadline not respected", elapsed)
	}
}

func TestSendMailContext_HappyPath(t *testing.T) {
	// Minimal fake SMTP server that accepts one message.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	received := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		fmt.Fprintf(conn, "220 localhost ESMTP\r\n")
		scanner := bufio.NewScanner(conn)
		var dataMode bool
		var body strings.Builder
		for scanner.Scan() {
			line := scanner.Text()
			if dataMode {
				if line == "." {
					dataMode = false
					fmt.Fprintf(conn, "250 OK\r\n")
					continue
				}
				body.WriteString(line + "\n")
				continue
			}
			switch {
			case strings.HasPrefix(line, "EHLO"):
				fmt.Fprintf(conn, "250-localhost\r\n250 OK\r\n")
			case strings.HasPrefix(line, "MAIL FROM"):
				fmt.Fprintf(conn, "250 OK\r\n")
			case strings.HasPrefix(line, "RCPT TO"):
				fmt.Fprintf(conn, "250 OK\r\n")
			case line == "DATA":
				fmt.Fprintf(conn, "354 Go ahead\r\n")
				dataMode = true
			case line == "QUIT":
				fmt.Fprintf(conn, "221 Bye\r\n")
				received <- body.String()
				return
			default:
				fmt.Fprintf(conn, "250 OK\r\n")
			}
		}
	}()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = sendMailContext(ctx, "127.0.0.1", portStr, "", "", "from@test.com", []string{"to@test.com"}, []byte("Subject: test\r\n\r\nhello"))
	if err != nil {
		t.Fatalf("sendMailContext: %v", err)
	}

	select {
	case body := <-received:
		if !strings.Contains(body, "hello") {
			t.Errorf("expected body to contain 'hello', got: %s", body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for fake SMTP to receive message")
	}
}
