package importer

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

type KumaBackup struct {
	Version          string           `json:"version"`
	MonitorList      []KumaMonitor    `json:"monitorList"`
	NotificationList []KumaNotifEntry `json:"notificationList"`
}

type KumaMonitor struct {
	ID               int               `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	URL              string            `json:"url"`
	Hostname         string            `json:"hostname"`
	Port             int               `json:"port"`
	Type             string            `json:"type"`
	Interval         int               `json:"interval"`
	Timeout          int               `json:"timeout"`
	MaxRetries       int               `json:"maxretries"`
	Method           string            `json:"method"`
	AcceptedCodes    []string          `json:"accepted_statuscodes"`
	IgnoreTLS        bool              `json:"ignoreTls"`
	Parent           int               `json:"parent"`
	Active           bool              `json:"active"`
	DNSResolveType   string            `json:"dns_resolve_type"`
	DNSResolveServer string            `json:"dns_resolve_server"`
	NotificationIDs  map[string]bool   `json:"notificationIDList"`
	ExpiryNotif      bool              `json:"expiryNotification"`
	Tags             []json.RawMessage `json:"tags"`
}

type KumaNotifEntry struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Config string `json:"config"`
}

type KumaNotifConfig struct {
	Type     string `json:"type"`
	URL      string `json:"ntfyserverurl"`
	Topic    string `json:"ntfytopic"`
	Priority int    `json:"ntfyPriority"`
	AuthMode string `json:"ntfyAuthenticationMethod"`
	Username string `json:"ntfyusername"`
	Password string `json:"ntfypassword"`
}

func LoadKumaFile(path string) (*KumaBackup, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	var backup KumaBackup
	if err := json.Unmarshal(data, &backup); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}
	return &backup, nil
}

func ConvertKuma(kb *KumaBackup) models.Backup {
	alertMap := convertKumaNotifications(kb.NotificationList)

	var alerts []models.AlertConfig
	for _, a := range alertMap {
		alerts = append(alerts, a)
	}

	kumaToUpkeepAlert := make(map[int]int)
	for _, n := range kb.NotificationList {
		if a, ok := alertMap[n.ID]; ok {
			kumaToUpkeepAlert[n.ID] = a.ID
		}
	}

	var sites []models.SiteConfig
	for _, m := range kb.MonitorList {
		site := convertKumaMonitor(m, kumaToUpkeepAlert)
		sites = append(sites, site)
	}

	return models.Backup{
		Sites:  sites,
		Alerts: alerts,
	}
}

func convertKumaNotifications(entries []KumaNotifEntry) map[int]models.AlertConfig {
	result := make(map[int]models.AlertConfig)
	for _, entry := range entries {
		var cfg KumaNotifConfig
		if err := json.Unmarshal([]byte(entry.Config), &cfg); err != nil {
			continue
		}

		alert := models.AlertConfig{
			ID:       entry.ID,
			Name:     entry.Name,
			Settings: make(map[string]string),
		}

		switch cfg.Type {
		case "ntfy":
			alert.Type = "ntfy"
			alert.Settings["url"] = strings.TrimRight(cfg.URL, "/")
			alert.Settings["topic"] = cfg.Topic
			alert.Settings["priority"] = fmt.Sprintf("%d", cfg.Priority)
			if cfg.AuthMode == "usernamePassword" {
				alert.Settings["username"] = cfg.Username
				alert.Settings["password"] = cfg.Password
			}
		case "discord":
			alert.Type = "discord"
			alert.Settings["url"] = cfg.URL
		case "slack":
			alert.Type = "slack"
			alert.Settings["url"] = cfg.URL
		default:
			alert.Type = "webhook"
			alert.Settings["url"] = cfg.URL
		}

		result[entry.ID] = alert
	}
	return result
}

func convertKumaMonitor(m KumaMonitor, alertMap map[int]int) models.SiteConfig {
	site := models.SiteConfig{
		ID:          m.ID,
		Name:        m.Name,
		Description: m.Description,
		Type:        m.Type,
		Interval:    m.Interval,
		Timeout:     m.Timeout,
		MaxRetries:  m.MaxRetries,
		Method:      m.Method,
		Hostname:    m.Hostname,
		Port:        m.Port,
		IgnoreTLS:   m.IgnoreTLS,
		ParentID:    m.Parent,
	}

	if len(m.AcceptedCodes) > 0 {
		site.AcceptedCodes = strings.Join(m.AcceptedCodes, ",")
	}

	site.DNSResolveType = m.DNSResolveType
	site.DNSServer = m.DNSResolveServer

	switch m.Type {
	case "http":
		site.URL = m.URL
		site.CheckSSL = m.ExpiryNotif
	case "ping":
		if m.Hostname != "" {
			site.Hostname = m.Hostname
		}
	case "port":
		if m.Hostname != "" {
			site.Hostname = m.Hostname
		}
	case "dns":
		if m.Hostname != "" {
			site.Hostname = m.Hostname
		}
	case "group":
		// groups are organizational only
	}

	for nidStr := range m.NotificationIDs {
		var nid int
		_, _ = fmt.Sscanf(nidStr, "%d", &nid) //nolint:errcheck
		if upkeepID, ok := alertMap[nid]; ok {
			site.AlertID = upkeepID
			break
		}
	}

	return site
}
