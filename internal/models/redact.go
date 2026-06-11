package models

// safeAlertSettingKeys lists, per provider type, the alert settings that are
// NOT secret and may be shown or exported in the clear. Everything else is
// redacted. Providers absent from this map (discord, slack, webhook, pushover)
// carry their secret in a field a denylist would miss — the webhook URL, the
// pushover token/user — so all of their settings are redacted.
var safeAlertSettingKeys = map[string]map[string]bool{
	"email":     {"host": true, "port": true, "to": true, "from": true},
	"ntfy":      {"topic": true, "priority": true},
	"telegram":  {"chat_id": true},
	"pagerduty": {"severity": true},
	"gotify":    {"priority": true},
	"opsgenie":  {"priority": true, "eu": true},
}

// RedactAlertSettings keeps only the known-safe keys for the alert type and
// redacts everything else. An allowlist fails safe: an unknown or newly added
// setting is redacted by default instead of leaking. Shared by the backup
// export path and the TUI alert detail panel so both render through the same
// policy.
func RedactAlertSettings(alertType string, settings map[string]string) map[string]string {
	safe := safeAlertSettingKeys[alertType]
	redacted := make(map[string]string, len(settings))
	for k, v := range settings {
		switch {
		case v == "":
			redacted[k] = ""
		case safe[k]:
			redacted[k] = v
		default:
			redacted[k] = "***REDACTED***"
		}
	}
	return redacted
}
