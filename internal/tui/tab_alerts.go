package tui

import (
	"fmt"
	"strings"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/monitor"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

type alertFormData struct {
	Name       string
	AlertType  string
	WebhookURL string
	SMTPHost   string
	SMTPPort   string
	SMTPUser   string
	SMTPPass   string
	EmailFrom  string
	EmailTo    string
	NtfyURL    string
	NtfyTopic  string
	NtfyUser   string
	NtfyPass   string
	NtfyPri    string
	// Telegram
	TelegramToken  string
	TelegramChatID string
	// PagerDuty
	PagerDutyKey      string
	PagerDutySeverity string
	// Pushover
	PushoverToken string
	PushoverUser  string
	// Gotify
	GotifyURL      string
	GotifyToken    string
	GotifyPriority string
	// Opsgenie
	OpsgenieAPIKey   string
	OpsgeniePriority string
	OpsgenieEU       bool
}

func fmtAlertType(t string) string {
	switch t {
	case "discord":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#5865F2")).Render(t)
	case "slack":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#E01E5A")).Render(t)
	case "webhook":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#F0E442")).Render(t)
	case "email":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#73F59F")).Render(t)
	case "ntfy":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")).Render(t)
	case "telegram":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#26A5E4")).Render(t)
	case "pagerduty":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#06AC38")).Render(t)
	case "pushover":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#249DF1")).Render(t)
	case "gotify":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#3F8BBA")).Render(t)
	case "opsgenie":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#2684FF")).Render(t)
	default:
		return t
	}
}

func fmtAlertConfig(alert struct {
	Type     string
	Settings map[string]string
}) string {
	switch alert.Type {
	case "email":
		host := alert.Settings["host"]
		to := alert.Settings["to"]
		if host != "" && to != "" {
			return limitStr(fmt.Sprintf("%s → %s", host, to), 34)
		}
		if host != "" {
			return limitStr(host, 34)
		}
		return subtleStyle.Render("—")
	case "ntfy":
		topic := alert.Settings["topic"]
		url := alert.Settings["url"]
		if url != "" && topic != "" {
			return limitStr(fmt.Sprintf("%s/%s", url, topic), 34)
		}
		return subtleStyle.Render("—")
	case "telegram":
		if id := alert.Settings["chat_id"]; id != "" {
			return limitStr(fmt.Sprintf("chat:%s", id), 34)
		}
		return subtleStyle.Render("—")
	case "pagerduty":
		if key := alert.Settings["routing_key"]; key != "" {
			return limitStr(key, 34)
		}
		return subtleStyle.Render("—")
	case "pushover":
		if user := alert.Settings["user"]; user != "" {
			return limitStr(fmt.Sprintf("user:%s", user), 34)
		}
		return subtleStyle.Render("—")
	case "gotify":
		if url := alert.Settings["url"]; url != "" {
			return limitStr(url, 34)
		}
		return subtleStyle.Render("—")
	case "opsgenie":
		key := alert.Settings["api_key"]
		if key != "" {
			masked := key
			if len(masked) > 8 {
				masked = masked[:4] + "…" + masked[len(masked)-4:]
			}
			if alert.Settings["eu"] == "true" {
				return limitStr(fmt.Sprintf("EU %s", masked), 34)
			}
			return limitStr(masked, 34)
		}
		return subtleStyle.Render("—")
	default:
		if val, ok := alert.Settings["url"]; ok {
			return limitStr(val, 34)
		}
		return subtleStyle.Render("—")
	}
}

func fmtAlertHealth(h monitor.AlertHealth) string {
	if h.LastSendAt.IsZero() {
		return subtleStyle.Render("●")
	}
	if h.LastSendOK {
		return specialStyle.Render("●")
	}
	return dangerStyle.Render("●")
}

func fmtAlertLastSent(h monitor.AlertHealth) string {
	if h.LastSendAt.IsZero() {
		return subtleStyle.Render("never")
	}
	d := time.Since(h.LastSendAt)
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours())/24)
}

func (m Model) viewAlertsTab() string {
	if len(m.alerts) == 0 {
		return "\n  No alert channels configured. Press [n] to add one."
	}

	var headers []string
	var widths []int
	if m.isWide() {
		headers = []string{"#", "", "NAME", "TYPE", "CONFIG", "LAST SENT"}
		widths = []int{4, 3, 18, 12, 40, 12}
	} else {
		headers = []string{"#", "", "NAME", "TYPE", "CONFIG", "SENT"}
		widths = []int{4, 3, 14, 10, 24, 8}
	}
	nameW := widths[2]
	cfgW := widths[4]

	return m.renderTable(
		headers,
		len(m.alerts),
		func(start, end int) [][]string {
			var rows [][]string
			for i := start; i < end; i++ {
				a := m.alerts[i]
				h := m.engine.GetAlertHealth(a.ID)
				rows = append(rows, []string{
					fmt.Sprintf("%d", i+1),
					fmtAlertHealth(h),
					m.zones.Mark(fmt.Sprintf("alert-%d", i), limitStr(a.Name, nameW-2)),
					fmtAlertType(a.Type),
					limitStr(fmtAlertConfig(struct {
						Type     string
						Settings map[string]string
					}{a.Type, a.Settings}), cfgW-2),
					fmtAlertLastSent(h),
				})
			}
			return rows
		},
		widths, nil,
	)
}

func (m Model) viewAlertDetailPanel() string {
	if m.cursor >= len(m.alerts) {
		return ""
	}
	a := m.alerts[m.cursor]
	h := m.engine.GetAlertHealth(a.ID)

	var b strings.Builder

	b.WriteString(subtleStyle.Render("  Alerts > ") + titleStyle.Render(a.Name) + "\n\n")

	row := func(label, value string) {
		fmt.Fprintf(&b, "  %-16s %s\n", subtleStyle.Render(label), value)
	}

	row("Type", fmtAlertType(a.Type))

	if h.LastSendAt.IsZero() {
		row("Health", subtleStyle.Render("never sent"))
	} else if h.LastSendOK {
		row("Health", specialStyle.Render("OK"))
	} else {
		row("Health", dangerStyle.Render("FAILED"))
	}

	if !h.LastSendAt.IsZero() {
		row("Last Sent", h.LastSendAt.Format("2006-01-02 15:04:05")+" ("+fmtAlertLastSent(h)+")")
	}
	if h.SendCount > 0 {
		row("Sends", fmt.Sprintf("%d sent, %d failed", h.SendCount, h.FailCount))
	}
	if h.LastError != "" {
		row("Last Error", dangerStyle.Render(limitStr(h.LastError, 60)))
	}

	b.WriteString("\n" + subtleStyle.Render("  CONFIGURATION") + "\n")
	for k, v := range a.Settings {
		row(k, v)
	}

	b.WriteString("\n\n")
	b.WriteString(subtleStyle.Render("  [i/Esc] Back  [e] Edit  [t] Test  [q] Quit"))

	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}

func (m *Model) initAlertHuhForm() tea.Cmd {
	m.alertFormData = &alertFormData{
		AlertType:         "discord",
		NtfyPri:           "3",
		PagerDutySeverity: "critical",
		GotifyPriority:    "5",
		OpsgeniePriority:  "P3",
	}

	if m.editID > 0 {
		for _, alert := range m.alerts {
			if alert.ID == m.editID {
				m.alertFormData.Name = alert.Name
				m.alertFormData.AlertType = alert.Type
				if url, ok := alert.Settings["url"]; ok {
					m.alertFormData.WebhookURL = url
				}
				switch alert.Type {
				case "email":
					m.alertFormData.SMTPHost = alert.Settings["host"]
					m.alertFormData.SMTPPort = alert.Settings["port"]
					m.alertFormData.SMTPUser = alert.Settings["user"]
					m.alertFormData.SMTPPass = alert.Settings["pass"]
					m.alertFormData.EmailFrom = alert.Settings["from"]
					m.alertFormData.EmailTo = alert.Settings["to"]
				case "ntfy":
					m.alertFormData.NtfyURL = alert.Settings["url"]
					m.alertFormData.NtfyTopic = alert.Settings["topic"]
					m.alertFormData.NtfyUser = alert.Settings["username"]
					m.alertFormData.NtfyPass = alert.Settings["password"]
					m.alertFormData.NtfyPri = alert.Settings["priority"]
				case "telegram":
					m.alertFormData.TelegramToken = alert.Settings["token"]
					m.alertFormData.TelegramChatID = alert.Settings["chat_id"]
				case "pagerduty":
					m.alertFormData.PagerDutyKey = alert.Settings["routing_key"]
					m.alertFormData.PagerDutySeverity = alert.Settings["severity"]
				case "pushover":
					m.alertFormData.PushoverToken = alert.Settings["token"]
					m.alertFormData.PushoverUser = alert.Settings["user"]
				case "gotify":
					m.alertFormData.GotifyURL = alert.Settings["url"]
					m.alertFormData.GotifyToken = alert.Settings["token"]
					m.alertFormData.GotifyPriority = alert.Settings["priority"]
				case "opsgenie":
					m.alertFormData.OpsgenieAPIKey = alert.Settings["api_key"]
					m.alertFormData.OpsgeniePriority = alert.Settings["priority"]
					m.alertFormData.OpsgenieEU = alert.Settings["eu"] == "true"
				}
				break
			}
		}
	}

	m.huhForm = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Alert Name").
				Placeholder("My Alert Channel").
				Value(&m.alertFormData.Name).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("name is required")
					}
					return nil
				}),
			huh.NewSelect[string]().Title("Alert Type").
				Options(
					huh.NewOption("Discord", "discord"),
					huh.NewOption("Slack", "slack"),
					huh.NewOption("Webhook", "webhook"),
					huh.NewOption("Email (SMTP)", "email"),
					huh.NewOption("Ntfy", "ntfy"),
					huh.NewOption("Telegram", "telegram"),
					huh.NewOption("PagerDuty", "pagerduty"),
					huh.NewOption("Pushover", "pushover"),
					huh.NewOption("Gotify", "gotify"),
					huh.NewOption("Opsgenie", "opsgenie"),
				).Value(&m.alertFormData.AlertType),
		).Title("Alert Config"),
		huh.NewGroup(
			huh.NewInput().Title("Webhook URL").
				Placeholder("https://discord.com/api/webhooks/...").
				Value(&m.alertFormData.WebhookURL),
		).Title("Webhook").WithHideFunc(func() bool {
			t := m.alertFormData.AlertType
			return t != "discord" && t != "slack" && t != "webhook"
		}),
		huh.NewGroup(
			huh.NewInput().Title("Ntfy Server URL").
				Placeholder("https://ntfy.sh").
				Value(&m.alertFormData.NtfyURL),
			huh.NewInput().Title("Topic").
				Placeholder("my-alerts").
				Value(&m.alertFormData.NtfyTopic),
			huh.NewSelect[string]().Title("Priority").
				Options(
					huh.NewOption("Min (1)", "1"),
					huh.NewOption("Low (2)", "2"),
					huh.NewOption("Default (3)", "3"),
					huh.NewOption("High (4)", "4"),
					huh.NewOption("Urgent (5)", "5"),
				).Value(&m.alertFormData.NtfyPri),
			huh.NewInput().Title("Username (optional)").
				Placeholder("admin").
				Value(&m.alertFormData.NtfyUser),
			huh.NewInput().Title("Password (optional)").
				EchoMode(huh.EchoModePassword).
				Value(&m.alertFormData.NtfyPass),
		).Title("Ntfy Settings").WithHideFunc(func() bool {
			return m.alertFormData.AlertType != "ntfy"
		}),
		huh.NewGroup(
			huh.NewInput().Title("SMTP Host").
				Placeholder("smtp.gmail.com").
				Value(&m.alertFormData.SMTPHost),
			huh.NewInput().Title("SMTP Port").
				Placeholder("587").
				Value(&m.alertFormData.SMTPPort),
			huh.NewInput().Title("SMTP User").
				Placeholder("user@gmail.com").
				Value(&m.alertFormData.SMTPUser),
			huh.NewInput().Title("SMTP Password").
				EchoMode(huh.EchoModePassword).
				Value(&m.alertFormData.SMTPPass),
			huh.NewInput().Title("From Email").
				Placeholder("alerts@domain.com").
				Value(&m.alertFormData.EmailFrom),
			huh.NewInput().Title("To Email").
				Placeholder("oncall@domain.com").
				Value(&m.alertFormData.EmailTo),
		).Title("Email Settings").WithHideFunc(func() bool {
			return m.alertFormData.AlertType != "email"
		}),
		huh.NewGroup(
			huh.NewInput().Title("Bot Token").
				Placeholder("123456:ABC-DEF1234...").
				Value(&m.alertFormData.TelegramToken),
			huh.NewInput().Title("Chat ID").
				Placeholder("-1001234567890").
				Value(&m.alertFormData.TelegramChatID),
		).Title("Telegram Settings").WithHideFunc(func() bool {
			return m.alertFormData.AlertType != "telegram"
		}),
		huh.NewGroup(
			huh.NewInput().Title("Routing Key").
				Placeholder("your-integration-routing-key").
				Value(&m.alertFormData.PagerDutyKey),
			huh.NewSelect[string]().Title("Severity").
				Options(
					huh.NewOption("Critical", "critical"),
					huh.NewOption("Error", "error"),
					huh.NewOption("Warning", "warning"),
					huh.NewOption("Info", "info"),
				).Value(&m.alertFormData.PagerDutySeverity),
		).Title("PagerDuty Settings").WithHideFunc(func() bool {
			return m.alertFormData.AlertType != "pagerduty"
		}),
		huh.NewGroup(
			huh.NewInput().Title("App Token").
				Placeholder("your-pushover-app-token").
				Value(&m.alertFormData.PushoverToken),
			huh.NewInput().Title("User Key").
				Placeholder("your-pushover-user-key").
				Value(&m.alertFormData.PushoverUser),
		).Title("Pushover Settings").WithHideFunc(func() bool {
			return m.alertFormData.AlertType != "pushover"
		}),
		huh.NewGroup(
			huh.NewInput().Title("Server URL").
				Placeholder("https://gotify.example.com").
				Value(&m.alertFormData.GotifyURL),
			huh.NewInput().Title("App Token").
				Placeholder("your-gotify-app-token").
				Value(&m.alertFormData.GotifyToken),
			huh.NewSelect[string]().Title("Priority").
				Options(
					huh.NewOption("Min (0)", "0"),
					huh.NewOption("Low (2)", "2"),
					huh.NewOption("Normal (5)", "5"),
					huh.NewOption("High (8)", "8"),
				).Value(&m.alertFormData.GotifyPriority),
		).Title("Gotify Settings").WithHideFunc(func() bool {
			return m.alertFormData.AlertType != "gotify"
		}),
		huh.NewGroup(
			huh.NewInput().Title("API Key").
				Placeholder("your-opsgenie-api-key").
				Value(&m.alertFormData.OpsgenieAPIKey),
			huh.NewSelect[string]().Title("Priority").
				Options(
					huh.NewOption("Critical (P1)", "P1"),
					huh.NewOption("High (P2)", "P2"),
					huh.NewOption("Moderate (P3)", "P3"),
					huh.NewOption("Low (P4)", "P4"),
					huh.NewOption("Informational (P5)", "P5"),
				).Value(&m.alertFormData.OpsgeniePriority),
			huh.NewConfirm().Title("EU Instance?").
				Value(&m.alertFormData.OpsgenieEU),
		).Title("Opsgenie Settings").WithHideFunc(func() bool {
			return m.alertFormData.AlertType != "opsgenie"
		}),
	).WithTheme(m.theme.HuhTheme())

	return m.huhForm.Init()
}

func (m *Model) submitAlertForm() {
	d := m.alertFormData
	settings := make(map[string]string)

	switch d.AlertType {
	case "email":
		settings["host"] = d.SMTPHost
		settings["port"] = d.SMTPPort
		settings["user"] = d.SMTPUser
		settings["pass"] = d.SMTPPass
		settings["from"] = d.EmailFrom
		settings["to"] = d.EmailTo
	case "ntfy":
		settings["url"] = d.NtfyURL
		settings["topic"] = d.NtfyTopic
		settings["priority"] = d.NtfyPri
		settings["username"] = d.NtfyUser
		settings["password"] = d.NtfyPass
	case "telegram":
		settings["token"] = d.TelegramToken
		settings["chat_id"] = d.TelegramChatID
	case "pagerduty":
		settings["routing_key"] = d.PagerDutyKey
		settings["severity"] = d.PagerDutySeverity
	case "pushover":
		settings["token"] = d.PushoverToken
		settings["user"] = d.PushoverUser
	case "gotify":
		settings["url"] = d.GotifyURL
		settings["token"] = d.GotifyToken
		settings["priority"] = d.GotifyPriority
	case "opsgenie":
		settings["api_key"] = d.OpsgenieAPIKey
		settings["priority"] = d.OpsgeniePriority
		if d.OpsgenieEU {
			settings["eu"] = "true"
		}
	default:
		settings["url"] = d.WebhookURL
	}

	if m.editID > 0 {
		if err := m.store.UpdateAlert(m.editID, d.Name, d.AlertType, settings); err != nil {
			m.engine.AddLog("Update alert failed: " + err.Error())
		}
	} else {
		if err := m.store.AddAlert(d.Name, d.AlertType, settings); err != nil {
			m.engine.AddLog("Add alert failed: " + err.Error())
		}
	}
	m.state = stateDashboard
}
