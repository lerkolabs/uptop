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

type colKey int

const (
	colNum colKey = iota
	colName
	colType
	colStatus
	colLatency
	colUptime
	colHistory
	colSSL
	colRetries
)

type columnDef struct {
	key     colKey
	wide    string
	narrow  string
	wideW   int
	narrowW int
	minTerm int // minimum terminal width to show (0 = always)
}

var siteColumns = []columnDef{
	{colNum, "#", "#", 4, 4, 0},
	{colName, "NAME", "NAME", 0, 0, 0},
	{colType, "TYPE", "TYPE", 10, 8, mediumBreakpoint},
	{colStatus, "STATUS", "STATUS", 10, 10, 0},
	{colLatency, "LATENCY", "LAT", 10, 7, 0},
	{colUptime, "UPTIME", "UP%", 8, 8, mediumBreakpoint},
	{colHistory, "HISTORY", "HISTORY", 0, 0, mediumBreakpoint},
	{colSSL, "SSL", "SSL", 7, 5, wideBreakpoint},
	{colRetries, "RETRIES", "RT", 9, 5, wideBreakpoint},
}

type tableLayout struct {
	nameW, sparkW int
	headers       []string
	colWidths     []int
	active        []colKey
}

func (m Model) computeLayout() tableLayout {
	wide := m.isWide()

	var active []colKey
	var headers []string
	var widths []int
	var fixed int

	cw := m.contentWidth
	if cw == 0 {
		cw = m.termWidth
	}
	for _, c := range siteColumns {
		if c.minTerm > 0 && cw < c.minTerm {
			continue
		}
		active = append(active, c.key)
		if wide {
			headers = append(headers, c.wide)
			widths = append(widths, c.wideW)
			if c.wideW > 0 {
				fixed += c.wideW
			}
		} else {
			headers = append(headers, c.narrow)
			widths = append(widths, c.narrowW)
			if c.narrowW > 0 {
				fixed += c.narrowW
			}
		}
	}

	sortColMap := map[int]colKey{
		sortStatus:  colStatus,
		sortName:    colName,
		sortLatency: colLatency,
	}
	if sortedKey, ok := sortColMap[m.sortColumn]; ok {
		arrow := "▼"
		if m.sortAsc {
			arrow = "▲"
		}
		for i, k := range active {
			if k == sortedKey {
				headers[i] = headers[i] + arrow
				break
			}
		}
	}

	numCols := len(headers)
	borderOverhead := 2 + (numCols - 1)
	avail := cw - chromePadH - 2 - borderOverhead - fixed
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

	hasHistory := false
	for _, k := range active {
		if k == colHistory {
			hasHistory = true
			break
		}
	}

	var nameW, sparkW int
	if hasHistory {
		nameW = avail / 2
		sparkW = avail - nameW
	} else {
		nameW = avail
		sparkW = 0
	}

	if nameW > maxName {
		nameW = maxName
	}
	if nameW < 13 {
		nameW = 13
	}
	if nameW > 35 {
		nameW = 35
	}
	if sparkW > 0 {
		if sparkW < 15 {
			sparkW = 15
		}
		if sparkW > 62 {
			sparkW = 62
		}
	}

	for i, k := range active {
		if k == colName {
			widths[i] = nameW
		}
		if k == colHistory {
			widths[i] = sparkW
		}
	}

	return tableLayout{
		nameW:     nameW,
		sparkW:    sparkW,
		headers:   headers,
		colWidths: widths,
		active:    active,
	}
}

func pickCols(active []colKey, allCells map[colKey]string) []string {
	row := make([]string, len(active))
	for i, k := range active {
		row[i] = allCells[k]
	}
	return row
}

func (m Model) viewSitesTab() string {

	if len(m.sites) == 0 {
		return m.emptyState(m.st.titleStyle.Render("uptop")+"\n\nNo monitors configured yet.", "[n] Add your first monitor")
	}

	layout := m.computeLayout()
	nameW := layout.nameW
	sparkWidth := layout.sparkW - 2
	if sparkWidth < 8 {
		sparkWidth = 8
	}
	if sparkWidth > 60 {
		sparkWidth = 60
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
				rowIdx := i - start
				var rowBg lipgloss.TerminalColor
				if i == m.cursor {
					rowBg = m.theme.SelectedBg
				} else if rowIdx%2 == 1 {
					rowBg = m.theme.ZebraBg
				}

				if site.Type == "group" {
					groupRows[i-start] = true
					icon := typeIcon("group", m.collapsed[site.ID])
					cells := map[colKey]string{
						colNum:     strconv.Itoa(i + 1),
						colName:    m.zones.Mark(fmt.Sprintf("site-%d", i), icon+" "+limitStr(site.Name, nameW-4)),
						colType:    "group",
						colStatus:  m.fmtStatus(site.Status, site.Paused, m.isMonitorInMaintenance(site.ID)),
						colLatency: m.st.subtleStyle.Render("—"),
						colUptime:  m.groupUptime(site.ID),
						colHistory: m.groupSparkline(site.ID, sparkWidth, rowBg),
						colSSL:     m.st.subtleStyle.Render("-"),
						colRetries: m.st.subtleStyle.Render("—"),
					}
					rows = append(rows, pickCols(layout.active, cells))
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

				if (site.Status == models.StatusDown || site.Status == models.StatusSSLExp || site.Status == models.StatusLate || site.Status == models.StatusStale) && site.LastError != "" {
					nameLen := len([]rune(name))
					errSpace := nameW - nameLen - 3
					if errSpace > 10 {
						cat := classifyError(site.LastError, site.Type, site.StatusCode)
						tag := categoryTag(cat)
						errText := site.LastError
						if tag != "" {
							errText = tag + " " + errText
						}
						name = name + " " + m.st.subtleStyle.Render(limitStr(errText, errSpace))
					}
				}

				hist, _ := m.engine.GetHistory(site.ID)
				var spark string
				if site.Type == "push" {
					spark = m.heartbeatSparkline(hist.Statuses, sparkWidth, rowBg)
				} else {
					spark = m.latencySparkline(hist.Latencies, hist.Statuses, sparkWidth, rowBg)
				}

				cells := map[colKey]string{
					colNum:     strconv.Itoa(i + 1),
					colName:    m.zones.Mark(fmt.Sprintf("site-%d", i), name),
					colType:    typeIcon(site.Type, false) + " " + site.Type,
					colStatus:  m.fmtStatus(site.Status, site.Paused, m.isMonitorInMaintenance(site.ID)),
					colLatency: m.fmtLatency(site.Latency),
					colUptime:  m.fmtUptime(hist.Statuses),
					colHistory: spark,
					colSSL:     m.fmtSSL(site),
					colRetries: m.fmtRetries(site),
				}
				rows = append(rows, pickCols(layout.active, cells))
			}
			return rows
		},
		layout.colWidths,
		func(row, col int) *lipgloss.Style {
			if groupRows[row] {
				s := m.st.siteGroupStyle
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

	return m.rebuildSiteForm()
}

func (m *Model) rebuildSiteForm() tea.Cmd {
	groups := m.buildSiteFormGroups()
	m.huhForm = huh.NewForm(groups...).WithTheme(m.theme.HuhTheme())
	if m.termWidth > 0 {
		m.huhForm.WithWidth(m.termWidth)
	}
	formHeight := m.termHeight - 7
	if formHeight < 5 {
		formHeight = 5
	}
	m.huhForm.WithHeight(formHeight)
	m.lastSiteType = m.siteFormData.SiteType
	return m.huhForm.Init()
}

func (m *Model) siteFormOptions() (alertOpts, groupOpts []huh.Option[string]) {
	alertOpts = []huh.Option[string]{huh.NewOption("None", "0")}
	for _, a := range m.alerts {
		alertOpts = append(alertOpts, huh.NewOption(
			fmt.Sprintf("%s (%s)", a.Name, a.Type),
			strconv.Itoa(a.ID),
		))
	}
	groupOpts = []huh.Option[string]{huh.NewOption("None", "0")}
	for _, s := range m.sites {
		if s.Type == "group" && s.ID != m.editID {
			groupOpts = append(groupOpts, huh.NewOption(s.Name, strconv.Itoa(s.ID)))
		}
	}
	return
}

func (m *Model) buildSiteFormGroups() []*huh.Group {
	d := m.siteFormData
	alertOpts, groupOpts := m.siteFormOptions()

	// Page 1 — Monitor Setup: core fields + type-specific target
	setup := []huh.Field{
		huh.NewInput().Title("Monitor Name").
			Placeholder("My Service").
			Value(&d.Name).
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
			).Value(&d.SiteType),
		huh.NewSelect[string]().Title("Alert Channel").
			Options(alertOpts...).
			Value(&d.AlertID),
	}

	switch d.SiteType {
	case "http":
		setup = append(setup, huh.NewInput().Title("URL").
			Placeholder("https://example.com").
			Value(&d.URL).
			Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("URL is required")
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
			}))
	case "ping", "dns":
		setup = append(setup, huh.NewInput().Title("Hostname / IP").
			Placeholder("10.0.0.1").
			Value(&d.Hostname))
	case "port":
		setup = append(setup,
			huh.NewInput().Title("Hostname / IP").
				Placeholder("10.0.0.1").
				Value(&d.Hostname),
			huh.NewInput().Title("Port").
				Placeholder("443").
				Value(&d.Port).
				Validate(func(s string) error {
					v, err := strconv.Atoi(s)
					if err != nil {
						return fmt.Errorf("must be a number")
					}
					if v < 1 || v > 65535 {
						return fmt.Errorf("port must be 1-65535")
					}
					return nil
				}))
	}

	groups := []*huh.Group{huh.NewGroup(setup...).Title("Monitor Setup")}

	if d.SiteType == "group" {
		return groups
	}

	// Page 2 — Configuration: type-specific options + shared defaults
	var config []huh.Field

	if d.SiteType == "http" {
		config = append(config,
			huh.NewSelect[string]().Title("HTTP Method").
				Options(
					huh.NewOption("GET", "GET"),
					huh.NewOption("POST", "POST"),
					huh.NewOption("PUT", "PUT"),
					huh.NewOption("PATCH", "PATCH"),
					huh.NewOption("DELETE", "DELETE"),
					huh.NewOption("HEAD", "HEAD"),
					huh.NewOption("OPTIONS", "OPTIONS"),
				).Value(&d.Method),
			huh.NewInput().Title("Accepted Status Codes").
				Placeholder("200-299").
				Description("Ranges (200-299) and singles (301) separated by commas").
				Value(&d.AcceptedCodes),
		)
	}

	config = append(config,
		huh.NewInput().Title("Check Interval (seconds)").
			Placeholder("60").
			Value(&d.Interval).
			Validate(func(s string) error {
				v, err := strconv.Atoi(s)
				if err != nil {
					return fmt.Errorf("must be a number")
				}
				if v < 5 {
					return fmt.Errorf("minimum interval is 5 seconds")
				}
				return nil
			}),
		huh.NewInput().Title("Timeout (seconds)").
			Placeholder("5").
			Value(&d.Timeout).
			Validate(func(s string) error {
				v, err := strconv.Atoi(s)
				if err != nil {
					return fmt.Errorf("must be a number")
				}
				if v < 1 || v > 300 {
					return fmt.Errorf("timeout must be 1-300 seconds")
				}
				return nil
			}),
		huh.NewInput().Title("Max Retries Before Alert").
			Placeholder("0").
			Value(&d.Retries).
			Validate(func(s string) error {
				v, err := strconv.Atoi(s)
				if err != nil {
					return fmt.Errorf("must be a number")
				}
				if v < 0 {
					return fmt.Errorf("retries cannot be negative")
				}
				return nil
			}),
		huh.NewSelect[string]().Title("Parent Group").
			Options(groupOpts...).
			Value(&d.GroupID),
		huh.NewInput().Title("Description").
			Placeholder("Optional description").
			Value(&d.Description),
		huh.NewInput().Title("Probe Regions").
			Placeholder("us-east, eu-west (empty = all)").
			Description("Comma-separated regions for distributed probing").
			Value(&d.Regions),
	)

	if d.SiteType == "http" {
		config = append(config,
			huh.NewConfirm().Title("Monitor SSL Certificate?").
				Value(&d.CheckSSL),
			huh.NewInput().Title("SSL Warning Threshold (days)").
				Placeholder("7").
				Value(&d.Threshold).
				Validate(func(s string) error {
					if !d.CheckSSL {
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
			huh.NewConfirm().Title("Ignore TLS Errors?").
				Value(&d.IgnoreTLS),
		)
	}

	groups = append(groups, huh.NewGroup(config...).Title("Configuration"))
	return groups
}

func (m *Model) submitSiteForm() tea.Cmd {
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

	cfg := models.SiteConfig{
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

	st := m.store
	ctx := m.ctx
	m.state = stateDashboard
	if m.editID > 0 {
		m.engine.UpdateSiteConfig(cfg)
		return writeCmd("Update site", func() error { return st.UpdateSite(ctx, cfg) })
	}
	return writeCmd("Add site", func() error { return st.AddSite(ctx, cfg) })
}
