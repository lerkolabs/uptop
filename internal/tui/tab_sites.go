package tui

import (
	"fmt"
	"net/url"
	"strconv"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

var siteGroupStyle lipgloss.Style

type siteFormData struct {
	Name          string
	SiteType      string
	URL           string
	Method        string
	AcceptedCodes string
	Interval      string
	AlertID       string
	CheckSSL      bool
	Threshold     string
	Retries       string
	Hostname      string
	Port          string
	Timeout       string
	Description   string
	IgnoreTLS     bool
	GroupID       string
	Regions       string
}

type tableLayout struct {
	nameW, sparkW int
	headers       []string
	colWidths     []int
}

func (m Model) computeLayout() tableLayout {
	wide := m.isWide()

	var fixed int
	var headers []string
	var widths []int

	if wide {
		headers = []string{"#", "NAME", "TYPE", "STATUS", "LATENCY", "UPTIME", "HISTORY", "SSL", "RETRIES"}
		widths = []int{4, 0, 10, 10, 10, 8, 0, 7, 9}
		fixed = 4 + 10 + 10 + 10 + 8 + 7 + 9
	} else {
		headers = []string{"#", "NAME", "TYPE", "STATUS", "LAT", "UP%", "HISTORY", "SSL", "RT"}
		widths = []int{4, 0, 8, 10, 7, 8, 0, 5, 5}
		fixed = 4 + 8 + 10 + 7 + 8 + 5 + 5
	}

	numCols := len(headers)
	borderOverhead := 2 + (numCols - 1)
	avail := m.termWidth - chromePadH - 2 - borderOverhead - fixed
	if avail < 20 {
		avail = 20
	}

	maxName := 0
	for _, s := range m.sites {
		if n := len([]rune(s.Name)); n > maxName {
			maxName = n
		}
	}
	maxName += 4

	nameW := avail / 2
	if nameW > maxName {
		nameW = maxName
	}
	if nameW < 13 {
		nameW = 13
	}
	if nameW > 40 {
		nameW = 40
	}

	sparkW := avail - nameW
	if sparkW < 10 {
		sparkW = 10
	}

	widths[1] = nameW
	widths[6] = sparkW

	return tableLayout{
		nameW:     nameW,
		sparkW:    sparkW,
		headers:   headers,
		colWidths: widths,
	}
}

func (m Model) viewSitesTab() string {

	if len(m.sites) == 0 {
		welcome := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(m.theme.Accent).
			Padding(1, 3).
			Render(
				titleStyle.Render("uptop") + "\n\n" +
					"No monitors configured yet.\n\n" +
					subtleStyle.Render("[n] Add your first monitor"),
			)
		return "\n" + welcome
	}

	layout := m.computeLayout()
	nameW := layout.nameW
	sparkWidth := layout.sparkW - 2
	if sparkWidth < 8 {
		sparkWidth = 8
	}

	var groupRows map[int]bool
	return m.renderTable(
		layout.headers,
		len(m.sites),
		func(start, end int) [][]string {
			groupRows = make(map[int]bool)
			var rows [][]string
			for i := start; i < end; i++ {
				site := m.sites[i]

				if site.Type == "group" {
					groupRows[i-start] = true
					icon := typeIcon("group", m.collapsed[site.ID])
					rows = append(rows, []string{
						strconv.Itoa(i + 1),
						m.zones.Mark(fmt.Sprintf("site-%d", i), icon+" "+limitStr(site.Name, nameW-4)),
						"group",
						fmtStatus(site.Status, site.Paused, m.isMonitorInMaintenance(site.ID), ErrCatUnknown),
						subtleStyle.Render("—"),
						m.groupUptime(site.ID),
						m.groupSparkline(site.ID, sparkWidth),
						subtleStyle.Render("-"),
						subtleStyle.Render("—"),
					})
					continue
				}

				name := site.Name
				if site.ParentID > 0 {
					prefix := "├"
					if i+1 >= len(m.sites) || m.sites[i+1].ParentID != site.ParentID {
						prefix = "└"
					}
					name = prefix + " " + limitStr(name, nameW-4)
				} else {
					name = limitStr(name, nameW-2)
				}

				if (site.Status == "DOWN" || site.Status == "SSL EXP" || site.Status == "LATE") && site.LastError != "" {
					nameLen := len([]rune(name))
					errSpace := nameW - nameLen - 3
					if errSpace > 10 {
						cat := classifyError(site.LastError, site.Type, site.StatusCode)
						tag := categoryTag(cat)
						errText := site.LastError
						if tag != "" {
							errText = tag + " " + errText
						}
						name = name + " " + subtleStyle.Render(limitStr(errText, errSpace))
					}
				}

				hist, _ := m.engine.GetHistory(site.ID)
				var spark string
				if site.Type == "push" {
					spark = heartbeatSparkline(hist.Statuses, sparkWidth)
				} else {
					spark = latencySparkline(hist.Latencies, hist.Statuses, sparkWidth)
				}

				rows = append(rows, []string{
					strconv.Itoa(i + 1),
					m.zones.Mark(fmt.Sprintf("site-%d", i), name),
					typeIcon(site.Type, false) + " " + site.Type,
					fmtStatus(site.Status, site.Paused, m.isMonitorInMaintenance(site.ID), classifyError(site.LastError, site.Type, site.StatusCode)),
					fmtLatency(site.Latency),
					fmtUptime(hist.Statuses),
					spark,
					fmtSSL(site),
					fmtRetries(site),
				})
			}
			return rows
		},
		layout.colWidths,
		func(row, col int) *lipgloss.Style {
			if groupRows[row] {
				s := siteGroupStyle
				return &s
			}
			return nil
		},
	)
}

func (m *Model) initSiteHuhForm() tea.Cmd {
	m.siteFormData = &siteFormData{
		SiteType:      "http",
		Method:        "GET",
		AcceptedCodes: "200-299",
		Interval:      "60",
		Threshold:     "7",
		Retries:       "0",
		Timeout:       "5",
		Port:          "0",
		GroupID:       "0",
	}

	if m.editID > 0 {
		for _, site := range m.sites {
			if site.ID == m.editID {
				m.siteFormData.Name = site.Name
				m.siteFormData.SiteType = site.Type
				m.siteFormData.URL = site.URL
				m.siteFormData.Interval = strconv.Itoa(site.Interval)
				m.siteFormData.AlertID = strconv.Itoa(site.AlertID)
				m.siteFormData.CheckSSL = site.CheckSSL
				m.siteFormData.Threshold = strconv.Itoa(site.ExpiryThreshold)
				m.siteFormData.Retries = strconv.Itoa(site.MaxRetries)
				m.siteFormData.Hostname = site.Hostname
				m.siteFormData.Port = strconv.Itoa(site.Port)
				m.siteFormData.Timeout = strconv.Itoa(site.Timeout)
				m.siteFormData.Description = site.Description
				m.siteFormData.IgnoreTLS = site.IgnoreTLS
				m.siteFormData.GroupID = strconv.Itoa(site.ParentID)
				m.siteFormData.Method = site.Method
				m.siteFormData.AcceptedCodes = site.AcceptedCodes
				m.siteFormData.Regions = site.Regions
				break
			}
		}
	}

	alertOpts := []huh.Option[string]{huh.NewOption("None", "0")}
	if alerts, err := m.store.GetAllAlerts(); err == nil {
		for _, a := range alerts {
			alertOpts = append(alertOpts, huh.NewOption(
				fmt.Sprintf("%s (%s)", a.Name, a.Type),
				strconv.Itoa(a.ID),
			))
		}
	}

	groupOpts := []huh.Option[string]{huh.NewOption("None", "0")}
	for _, s := range m.sites {
		if s.Type == "group" && s.ID != m.editID {
			groupOpts = append(groupOpts, huh.NewOption(s.Name, strconv.Itoa(s.ID)))
		}
	}

	m.huhForm = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Monitor Name").
				Placeholder("My Service").
				Value(&m.siteFormData.Name).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("name is required")
					}
					return nil
				}),
			huh.NewSelect[string]().Title("Monitor Type").
				Options(
					huh.NewOption("HTTP/HTTPS", "http"),
					huh.NewOption("Push / Heartbeat", "push"),
					huh.NewOption("Ping (ICMP)", "ping"),
					huh.NewOption("TCP Port", "port"),
					huh.NewOption("DNS", "dns"),
					huh.NewOption("Group", "group"),
				).Value(&m.siteFormData.SiteType),
			huh.NewSelect[string]().Title("Alert Channel").
				Options(alertOpts...).
				Value(&m.siteFormData.AlertID),
		).Title("Monitor Settings"),
		huh.NewGroup(
			huh.NewInput().Title("URL").
				Placeholder("https://example.com").
				Description("Required for HTTP monitors").
				Value(&m.siteFormData.URL).
				Validate(func(s string) error {
					if m.siteFormData.SiteType != "http" {
						return nil
					}
					if s == "" {
						return fmt.Errorf("URL is required for HTTP monitors")
					}
					u, err := url.Parse(s)
					if err != nil {
						return fmt.Errorf("invalid URL")
					}
					if u.Scheme != "http" && u.Scheme != "https" {
						return fmt.Errorf("URL must start with http:// or https://")
					}
					if u.Host == "" {
						return fmt.Errorf("URL must include a host")
					}
					return nil
				}),
			huh.NewInput().Title("Check Interval (seconds)").
				Placeholder("60").
				Value(&m.siteFormData.Interval).
				Validate(func(s string) error {
					if m.siteFormData.SiteType == "group" {
						return nil
					}
					v, err := strconv.Atoi(s)
					if err != nil {
						return fmt.Errorf("must be a number")
					}
					if v < 5 {
						return fmt.Errorf("minimum interval is 5 seconds")
					}
					return nil
				}),
			huh.NewSelect[string]().Title("Parent Group").
				Options(groupOpts...).
				Value(&m.siteFormData.GroupID),
			huh.NewInput().Title("Hostname / IP").
				Placeholder("10.0.0.1").
				Description("Target for ping/port/DNS monitors").
				Value(&m.siteFormData.Hostname),
			huh.NewInput().Title("Port").
				Placeholder("0").
				Description("Target port for TCP port monitors").
				Value(&m.siteFormData.Port).
				Validate(func(s string) error {
					if m.siteFormData.SiteType != "port" {
						return nil
					}
					v, err := strconv.Atoi(s)
					if err != nil {
						return fmt.Errorf("must be a number")
					}
					if v < 1 || v > 65535 {
						return fmt.Errorf("port must be 1-65535")
					}
					return nil
				}),
			huh.NewInput().Title("Timeout (seconds)").
				Placeholder("5").
				Value(&m.siteFormData.Timeout).
				Validate(func(s string) error {
					if m.siteFormData.SiteType == "group" {
						return nil
					}
					v, err := strconv.Atoi(s)
					if err != nil {
						return fmt.Errorf("must be a number")
					}
					if v < 1 || v > 300 {
						return fmt.Errorf("timeout must be 1-300 seconds")
					}
					return nil
				}),
			huh.NewInput().Title("Description").
				Placeholder("Optional description").
				Value(&m.siteFormData.Description),
			huh.NewInput().Title("Probe Regions").
				Placeholder("us-east, eu-west (empty = all)").
				Description("Comma-separated regions for distributed probing").
				Value(&m.siteFormData.Regions),
		).Title("Connection").WithHideFunc(func() bool {
			return m.siteFormData.SiteType == "group"
		}),
		huh.NewGroup(
			huh.NewSelect[string]().Title("HTTP Method").
				Options(
					huh.NewOption("GET", "GET"),
					huh.NewOption("POST", "POST"),
					huh.NewOption("PUT", "PUT"),
					huh.NewOption("PATCH", "PATCH"),
					huh.NewOption("DELETE", "DELETE"),
					huh.NewOption("HEAD", "HEAD"),
					huh.NewOption("OPTIONS", "OPTIONS"),
				).Value(&m.siteFormData.Method),
			huh.NewInput().Title("Accepted Status Codes").
				Placeholder("200-299").
				Description("Ranges (200-299) and singles (301) separated by commas").
				Value(&m.siteFormData.AcceptedCodes),
		).Title("HTTP Settings").WithHideFunc(func() bool {
			return m.siteFormData.SiteType != "http"
		}),
		huh.NewGroup(
			huh.NewConfirm().Title("Monitor SSL Certificate?").
				Value(&m.siteFormData.CheckSSL),
			huh.NewInput().Title("SSL Warning Threshold (days)").
				Placeholder("7").
				Value(&m.siteFormData.Threshold).
				Validate(func(s string) error {
					if !m.siteFormData.CheckSSL {
						return nil
					}
					v, err := strconv.Atoi(s)
					if err != nil {
						return fmt.Errorf("must be a number")
					}
					if v < 1 {
						return fmt.Errorf("threshold must be at least 1 day")
					}
					return nil
				}),
			huh.NewInput().Title("Max Retries Before Alert").
				Placeholder("0").
				Value(&m.siteFormData.Retries).
				Validate(func(s string) error {
					if m.siteFormData.SiteType == "group" {
						return nil
					}
					v, err := strconv.Atoi(s)
					if err != nil {
						return fmt.Errorf("must be a number")
					}
					if v < 0 {
						return fmt.Errorf("retries cannot be negative")
					}
					return nil
				}),
			huh.NewConfirm().Title("Ignore TLS Errors?").
				Value(&m.siteFormData.IgnoreTLS),
		).Title("Advanced").WithHideFunc(func() bool {
			return m.siteFormData.SiteType == "group"
		}),
	).WithTheme(m.theme.HuhTheme())

	return m.huhForm.Init()
}

func (m *Model) submitSiteForm() {
	d := m.siteFormData
	interval, _ := strconv.Atoi(d.Interval)
	alertID, _ := strconv.Atoi(d.AlertID)
	threshold, _ := strconv.Atoi(d.Threshold)
	retries, _ := strconv.Atoi(d.Retries)
	port, _ := strconv.Atoi(d.Port)
	timeout, _ := strconv.Atoi(d.Timeout)
	groupID, _ := strconv.Atoi(d.GroupID)
	if interval < 1 {
		interval = 60
	}
	if threshold < 1 {
		threshold = 7
	}

	site := models.Site{
		ID:              m.editID,
		Name:            d.Name,
		URL:             d.URL,
		Type:            d.SiteType,
		Interval:        interval,
		AlertID:         alertID,
		CheckSSL:        d.CheckSSL,
		ExpiryThreshold: threshold,
		MaxRetries:      retries,
		Hostname:        d.Hostname,
		Port:            port,
		Timeout:         timeout,
		Description:     d.Description,
		IgnoreTLS:       d.IgnoreTLS,
		ParentID:        groupID,
		Method:          d.Method,
		AcceptedCodes:   d.AcceptedCodes,
		Regions:         d.Regions,
	}

	if m.editID > 0 {
		if err := m.store.UpdateSite(site); err != nil {
			m.engine.AddLog("Update site failed: " + err.Error())
		}
		m.engine.UpdateSiteConfig(site)
	} else {
		if err := m.store.AddSite(site); err != nil {
			m.engine.AddLog("Add site failed: " + err.Error())
		}
	}
	m.state = stateDashboard
}
