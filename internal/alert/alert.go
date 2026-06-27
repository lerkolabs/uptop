package alert

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

const alertHTTPTimeout = 10 * time.Second

var alertClient = &http.Client{Timeout: alertHTTPTimeout}

// sanitizeError strips the request URL from transport errors before they are
// stored or displayed. *url.Error embeds the full URL, which for several
// providers carries the credential itself (Telegram bot token in the path,
// webhook secrets in the URL). The operation and underlying cause — the useful
// diagnostic — are preserved.
func sanitizeError(err error) error {
	if err == nil {
		return nil
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return fmt.Errorf("%s request failed: %w", urlErr.Op, urlErr.Err)
	}
	return err
}

type Provider interface {
	Send(ctx context.Context, title, message string) error
}

type PayloadFunc func(title, message string) ([]byte, error)

type HTTPProvider struct {
	URL     string
	Payload PayloadFunc
	Headers map[string]string
}

func (h *HTTPProvider) Send(ctx context.Context, title, message string) error {
	body, err := h.Payload(title, message)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", h.URL, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range h.Headers {
		req.Header.Set(k, v)
	}
	resp, err := alertClient.Do(req)
	if err != nil {
		return sanitizeError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("alert webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func discordPayload(title, message string) ([]byte, error) {
	return json.Marshal(map[string]string{"content": fmt.Sprintf("**%s**\n%s", title, message)})
}

func slackPayload(title, message string) ([]byte, error) {
	return json.Marshal(map[string]string{"text": fmt.Sprintf("*%s*\n%s", title, message)})
}

func webhookPayload(title, message string) ([]byte, error) {
	return json.Marshal(map[string]string{"title": title, "message": message, "status": "alert"})
}

func telegramPayload(chatID string) PayloadFunc {
	return func(title, message string) ([]byte, error) {
		return json.Marshal(map[string]string{
			"chat_id":    chatID,
			"text":       fmt.Sprintf("*%s*\n%s", title, message),
			"parse_mode": "Markdown",
		})
	}
}

func pagerdutyPayload(routingKey, severity string) PayloadFunc {
	return func(title, message string) ([]byte, error) {
		return json.Marshal(map[string]any{
			"routing_key":  routingKey,
			"event_action": "trigger",
			"payload": map[string]string{
				"summary":  fmt.Sprintf("%s: %s", title, message),
				"source":   "uptop",
				"severity": severity,
			},
		})
	}
}

func pushoverPayload(token, user string) PayloadFunc {
	return func(title, message string) ([]byte, error) {
		return json.Marshal(map[string]string{
			"token":   token,
			"user":    user,
			"title":   title,
			"message": message,
		})
	}
}

func gotifyPayload(priority string) PayloadFunc {
	return func(title, message string) ([]byte, error) {
		pri, _ := strconv.Atoi(priority)
		return json.Marshal(map[string]any{
			"title":    title,
			"message":  message,
			"priority": pri,
		})
	}
}

func opsgeniePayload(priority string) PayloadFunc {
	return func(title, message string) ([]byte, error) {
		return json.Marshal(map[string]any{
			"message":     limitMessage(title, 130),
			"description": message,
			"source":      "uptop",
			"priority":    priority,
		})
	}
}

func limitMessage(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func GetProvider(cfg models.AlertConfig) Provider {
	switch cfg.Type {
	case "discord":
		return &HTTPProvider{URL: cfg.Settings["url"], Payload: discordPayload}
	case "slack":
		return &HTTPProvider{URL: cfg.Settings["url"], Payload: slackPayload}
	case "webhook":
		return &HTTPProvider{URL: cfg.Settings["url"], Payload: webhookPayload}
	case "email":
		port := "25"
		if p, ok := cfg.Settings["port"]; ok {
			port = p
		}
		return &EmailProvider{
			Host: cfg.Settings["host"],
			Port: port,
			User: cfg.Settings["user"],
			Pass: cfg.Settings["pass"],
			To:   cfg.Settings["to"],
			From: cfg.Settings["from"],
		}
	case "ntfy":
		priority := "3"
		if p, ok := cfg.Settings["priority"]; ok && p != "" {
			priority = p
		}
		return &NtfyProvider{
			ServerURL: cfg.Settings["url"],
			Topic:     cfg.Settings["topic"],
			Priority:  priority,
			Username:  cfg.Settings["username"],
			Password:  cfg.Settings["password"],
		}
	case "telegram":
		return &HTTPProvider{
			URL:     fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", cfg.Settings["token"]),
			Payload: telegramPayload(cfg.Settings["chat_id"]),
		}
	case "pagerduty":
		severity := "critical"
		if s, ok := cfg.Settings["severity"]; ok && s != "" {
			severity = s
		}
		return &HTTPProvider{
			URL:     "https://events.pagerduty.com/v2/enqueue",
			Payload: pagerdutyPayload(cfg.Settings["routing_key"], severity),
		}
	case "pushover":
		return &HTTPProvider{
			URL:     "https://api.pushover.net/1/messages.json",
			Payload: pushoverPayload(cfg.Settings["token"], cfg.Settings["user"]),
		}
	case "gotify":
		priority := "5"
		if p, ok := cfg.Settings["priority"]; ok && p != "" {
			priority = p
		}
		serverURL := strings.TrimRight(cfg.Settings["url"], "/")
		return &HTTPProvider{
			URL:     serverURL + "/message",
			Payload: gotifyPayload(priority),
			Headers: map[string]string{"X-Gotify-Key": cfg.Settings["token"]},
		}
	case "opsgenie":
		priority := "P3"
		if p, ok := cfg.Settings["priority"]; ok && p != "" {
			priority = p
		}
		apiURL := "https://api.opsgenie.com/v2/alerts"
		if eu, ok := cfg.Settings["eu"]; ok && eu == "true" {
			apiURL = "https://api.eu.opsgenie.com/v2/alerts"
		}
		return &HTTPProvider{
			URL:     apiURL,
			Payload: opsgeniePayload(priority),
			Headers: map[string]string{"Authorization": "GenieKey " + cfg.Settings["api_key"]},
		}
	default:
		return nil
	}
}

type EmailProvider struct {
	Host, Port, User, Pass, To, From string
}

func sanitizeHeader(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	return s
}

func (e *EmailProvider) Send(ctx context.Context, title, message string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	to := sanitizeHeader(e.To)
	from := sanitizeHeader(e.From)
	subject := sanitizeHeader(title)
	body := strings.ReplaceAll(message, "\r", "")
	msg := []byte("From: " + from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: uptop: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		body + "\r\n")
	return sendMailContext(ctx, e.Host, e.Port, e.User, e.Pass, from, []string{to}, msg)
}

// sendMailContext is a ctx-aware replacement for smtp.SendMail.
// smtp.SendMail ignores context entirely — a blackholed SMTP server hangs for
// the OS TCP timeout (minutes). This dials with the context deadline and sets
// connection deadlines so cancellation is respected throughout.
func sendMailContext(ctx context.Context, host, port, user, pass, from string, rcpt []string, msg []byte) error {
	addr := host + ":" + port

	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("smtp client: %w", err)
	}
	defer c.Close()

	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}

	if user != "" || pass != "" {
		auth := smtp.PlainAuth("", user, pass, host)
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	if err := c.Mail(from); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
	for _, r := range rcpt {
		if err := c.Rcpt(r); err != nil {
			return fmt.Errorf("smtp rcpt: %w", err)
		}
	}

	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp data close: %w", err)
	}

	return c.Quit()
}

type NtfyProvider struct {
	ServerURL string
	Topic     string
	Priority  string
	Username  string
	Password  string
}

func (n *NtfyProvider) Send(ctx context.Context, title, message string) error {
	url := strings.TrimRight(n.ServerURL, "/") + "/" + n.Topic
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(message))
	if err != nil {
		return err
	}
	req.Header.Set("Title", title)
	req.Header.Set("Priority", n.Priority)
	if n.Username != "" && n.Password != "" {
		req.SetBasicAuth(n.Username, n.Password)
	}
	resp, err := alertClient.Do(req)
	if err != nil {
		return sanitizeError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("ntfy returned HTTP %d", resp.StatusCode)
	}
	return nil
}
