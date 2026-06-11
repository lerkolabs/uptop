package tui

import (
	"fmt"
	"strconv"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

type maintFormData struct {
	Title       string
	Description string
	Type        string
	MonitorID   string
	Duration    string
	CustomHours string
}

func (m Model) fmtMaintStatus(mw models.MaintenanceWindow) string {
	now := time.Now()
	if mw.StartTime.After(now) {
		return m.st.warnStyle.Render("SCHEDULED")
	}
	if !mw.EndTime.IsZero() && mw.EndTime.Before(now) {
		return m.st.subtleStyle.Render("ENDED")
	}
	return m.st.specialStyle.Render("ACTIVE")
}

func (m Model) fmtMaintType(t string) string {
	if t == "incident" {
		return m.st.dangerStyle.Render("incident")
	}
	return m.st.maintStyle.Render("maintenance")
}

func fmtMaintMonitorW(monitorID int, sites []models.Site, maxW int) string {
	if monitorID == 0 {
		return "All"
	}
	for _, s := range sites {
		if s.ID == monitorID {
			return limitStr(s.Name, maxW)
		}
	}
	return fmt.Sprintf("#%d", monitorID)
}

func (m Model) fmtMaintTime(t time.Time, colW int) string {
	if t.IsZero() {
		return m.st.subtleStyle.Render("—")
	}
	now := time.Now()
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return t.Format("15:04")
	}
	if colW >= 14 {
		return t.Format("15:04 Jan 02")
	}
	return t.Format("Jan 02")
}

func (m Model) isMonitorInMaintenance(monitorID int) bool {
	for _, mw := range m.maintenanceWindows {
		if mw.Type != "maintenance" {
			continue
		}
		now := time.Now()
		if mw.StartTime.After(now) {
			continue
		}
		if !mw.EndTime.IsZero() && mw.EndTime.Before(now) {
			continue
		}
		if mw.MonitorID == 0 || mw.MonitorID == monitorID {
			return true
		}
		for _, s := range m.sites {
			if s.ID == monitorID && s.ParentID > 0 && mw.MonitorID == s.ParentID {
				return true
			}
		}
	}
	return false
}

func (m Model) viewMaintTab() string {
	if len(m.maintenanceWindows) == 0 {
		return m.emptyState("No maintenance windows or incidents.", "[n] Create one")
	}

	var headers []string
	var widths []int
	if m.isWide() {
		headers = []string{"#", "TITLE", "TYPE", "MONITORS", "STATUS", "STARTED", "ENDS"}
		widths = []int{4, 24, 14, 22, 12, 16, 16}
	} else {
		headers = []string{"#", "TITLE", "TYPE", "MON", "ST", "START", "ENDS"}
		widths = []int{4, 14, 13, 14, 11, 14, 14}
	}
	titleW := widths[1]
	monW := widths[3]
	timeW := widths[5]

	return m.renderTable(
		headers,
		len(m.maintenanceWindows),
		func(start, end int) [][]string {
			var rows [][]string
			allSites := m.engine.GetAllSites()
			for i := start; i < end; i++ {
				mw := m.maintenanceWindows[i]
				rows = append(rows, []string{
					strconv.Itoa(i + 1),
					m.zones.Mark(fmt.Sprintf("maint-%d", i), limitStr(mw.Title, titleW-2)),
					m.fmtMaintType(mw.Type),
					fmtMaintMonitorW(mw.MonitorID, allSites, monW-2),
					m.fmtMaintStatus(mw),
					m.fmtMaintTime(mw.StartTime, timeW),
					m.fmtMaintTime(mw.EndTime, timeW),
				})
			}
			return rows
		},
		widths,
		nil,
	)
}

func (m *Model) initMaintHuhForm() tea.Cmd {
	m.maintFormData = &maintFormData{
		Type:        "maintenance",
		MonitorID:   "0",
		Duration:    "1h",
		CustomHours: "12",
	}

	monitorOpts := []huh.Option[string]{huh.NewOption("All Monitors", "0")}
	allSites := m.engine.GetAllSites()
	for _, s := range allSites {
		label := s.Name
		if s.Type == "group" {
			label = s.Name + " (group)"
		}
		monitorOpts = append(monitorOpts, huh.NewOption(label, strconv.Itoa(s.ID)))
	}

	m.huhForm = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Title").
				Placeholder("DB Migration").
				Value(&m.maintFormData.Title).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("title is required")
					}
					return nil
				}),
			huh.NewSelect[string]().Title("Type").
				Options(
					huh.NewOption("Maintenance (suppress alerts)", "maintenance"),
					huh.NewOption("Incident (informational)", "incident"),
				).Value(&m.maintFormData.Type),
			huh.NewSelect[string]().Title("Affected Monitors").
				Options(monitorOpts...).
				Value(&m.maintFormData.MonitorID),
			huh.NewInput().Title("Description").
				Placeholder("Optional notes").
				Value(&m.maintFormData.Description),
		).Title("Maintenance Window"),
		huh.NewGroup(
			huh.NewSelect[string]().Title("Duration").
				Options(
					huh.NewOption("1 hour", "1h"),
					huh.NewOption("2 hours", "2h"),
					huh.NewOption("4 hours", "4h"),
					huh.NewOption("8 hours", "8h"),
					huh.NewOption("Indefinite (end manually)", "indefinite"),
					huh.NewOption("Custom", "custom"),
				).Value(&m.maintFormData.Duration),
			huh.NewInput().Title("Custom Duration (hours)").
				Placeholder("12").
				Value(&m.maintFormData.CustomHours).
				Validate(func(s string) error {
					if m.maintFormData.Duration != "custom" {
						return nil
					}
					v, err := strconv.Atoi(s)
					if err != nil {
						return fmt.Errorf("must be a number")
					}
					if v < 1 {
						return fmt.Errorf("must be at least 1 hour")
					}
					return nil
				}),
		).Title("Duration").WithHideFunc(func() bool {
			return m.maintFormData.Type == "incident"
		}),
	).WithTheme(m.theme.HuhTheme())

	return m.huhForm.Init()
}

func (m *Model) submitMaintForm() tea.Cmd {
	d := m.maintFormData
	monitorID, _ := strconv.Atoi(d.MonitorID)

	mw := models.MaintenanceWindow{
		MonitorID:   monitorID,
		Title:       d.Title,
		Description: d.Description,
		Type:        d.Type,
		StartTime:   time.Now(),
	}

	if d.Type == "maintenance" {
		switch d.Duration {
		case "1h":
			mw.EndTime = mw.StartTime.Add(1 * time.Hour)
		case "2h":
			mw.EndTime = mw.StartTime.Add(2 * time.Hour)
		case "4h":
			mw.EndTime = mw.StartTime.Add(4 * time.Hour)
		case "8h":
			mw.EndTime = mw.StartTime.Add(8 * time.Hour)
		case "custom":
			hours, _ := strconv.Atoi(d.CustomHours)
			if hours < 1 {
				hours = 1
			}
			mw.EndTime = mw.StartTime.Add(time.Duration(hours) * time.Hour)
		}
	}

	st := m.store
	m.state = stateDashboard
	return writeCmd("Add maintenance window", func() error {
		return st.AddMaintenanceWindow(mw)
	})
}
