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
	ctx := m.ctx
	m.state = stateDashboard
	return writeCmd("Add maintenance window", func() error {
		overlaps, _ := st.GetOverlappingMaintenanceWindows(ctx, mw.MonitorID, mw.StartTime, mw.EndTime)
		if len(overlaps) > 0 {
			_ = st.SaveLog(ctx, fmt.Sprintf("Overlap: new window %q overlaps with existing %q", mw.Title, overlaps[0].Title))
		}
		return st.AddMaintenanceWindow(ctx, mw)
	})
}
