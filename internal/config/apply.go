package config

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/store"
)

type ApplyOpts struct {
	DryRun bool
	Prune  bool
}

type Change struct {
	Action  string
	Kind    string
	Name    string
	Details string
}

func Apply(ctx context.Context, s store.Store, f *File, opts ApplyOpts) ([]Change, error) {
	if err := Validate(f); err != nil {
		return nil, err
	}

	existingAlerts, err := s.GetAllAlerts(ctx)
	if err != nil {
		return nil, fmt.Errorf("load alerts: %w", err)
	}

	existingSites, err := s.GetSites(ctx)
	if err != nil {
		return nil, fmt.Errorf("load sites: %w", err)
	}

	existingAlertsByName := make(map[string]models.AlertConfig, len(existingAlerts))
	for _, a := range existingAlerts {
		existingAlertsByName[a.Name] = a
	}

	existingSitesByName := make(map[string]models.Site, len(existingSites))
	for _, s := range existingSites {
		existingSitesByName[s.Name] = s
	}

	var changes []Change

	alertMap := make(map[string]int)
	for _, ea := range existingAlerts {
		alertMap[ea.Name] = ea.ID
	}

	desiredAlertNames := make(map[string]bool, len(f.Alerts))
	for _, a := range f.Alerts {
		desiredAlertNames[a.Name] = true
		existing, exists := existingAlertsByName[a.Name]
		if !exists {
			changes = append(changes, Change{Action: "create", Kind: "alert", Name: a.Name, Details: a.Type})
			if !opts.DryRun {
				id, err := s.AddAlertReturningID(ctx, a.Name, a.Type, a.Settings)
				if err != nil {
					return changes, fmt.Errorf("create alert %q: %w", a.Name, err)
				}
				alertMap[a.Name] = id
			}
		} else {
			alertMap[a.Name] = existing.ID
			if diff := diffAlert(existing, a); diff != "" {
				changes = append(changes, Change{Action: "update", Kind: "alert", Name: a.Name, Details: diff})
				if !opts.DryRun {
					if err := s.UpdateAlert(ctx, existing.ID, a.Name, a.Type, a.Settings); err != nil {
						return changes, fmt.Errorf("update alert %q: %w", a.Name, err)
					}
				}
			}
		}
	}

	desiredMonitorNames := make(map[string]bool)
	collectMonitorNames(f.Monitors, desiredMonitorNames)

	var groups []Monitor
	var topLevel []Monitor
	for _, m := range f.Monitors {
		if m.Type == "group" {
			groups = append(groups, m)
		} else {
			topLevel = append(topLevel, m)
		}
	}

	groupMap := make(map[string]int)
	for _, g := range groups {
		alertID, err := resolveAlertID(alertMap, g.Alert)
		if err != nil {
			return changes, fmt.Errorf("monitor %q: %w", g.Name, err)
		}
		site := monitorToSite(g, alertID, 0)
		existing, exists := existingSitesByName[g.Name]
		if !exists {
			changes = append(changes, Change{Action: "create", Kind: "monitor", Name: g.Name, Details: "group"})
			if !opts.DryRun {
				id, err := s.AddSiteReturningID(ctx, site)
				if err != nil {
					return changes, fmt.Errorf("create group %q: %w", g.Name, err)
				}
				groupMap[g.Name] = id
			}
		} else {
			groupMap[g.Name] = existing.ID
			site.ID = existing.ID
			if diff := diffSite(normalizeSite(existing), site); diff != "" {
				changes = append(changes, Change{Action: "update", Kind: "monitor", Name: g.Name, Details: diff})
				if !opts.DryRun {
					if err := s.UpdateSite(ctx, site); err != nil {
						return changes, fmt.Errorf("update group %q: %w", g.Name, err)
					}
				}
			}
		}
	}

	for _, g := range groups {
		parentID := groupMap[g.Name]
		for _, child := range g.Monitors {
			c, err := applyMonitor(ctx, s, child, alertMap, existingSitesByName, parentID, opts.DryRun)
			if err != nil {
				return changes, err
			}
			changes = append(changes, c...)
		}
	}

	for _, m := range topLevel {
		c, err := applyMonitor(ctx, s, m, alertMap, existingSitesByName, 0, opts.DryRun)
		if err != nil {
			return changes, err
		}
		changes = append(changes, c...)
	}

	if opts.Prune {
		var childDeletes []Change
		var groupDeletes []Change
		for _, es := range existingSites {
			if desiredMonitorNames[es.Name] {
				continue
			}
			c := Change{Action: "delete", Kind: "monitor", Name: es.Name, Details: es.Type}
			if es.Type == "group" {
				groupDeletes = append(groupDeletes, c)
			} else {
				childDeletes = append(childDeletes, c)
			}
			if !opts.DryRun {
				if err := s.DeleteSite(ctx, es.ID); err != nil {
					return changes, fmt.Errorf("delete monitor %q: %w", es.Name, err)
				}
			}
		}
		changes = append(changes, childDeletes...)
		changes = append(changes, groupDeletes...)

		for _, ea := range existingAlerts {
			if desiredAlertNames[ea.Name] {
				continue
			}
			changes = append(changes, Change{Action: "delete", Kind: "alert", Name: ea.Name, Details: ea.Type})
			if !opts.DryRun {
				if err := s.DeleteAlert(ctx, ea.ID); err != nil {
					return changes, fmt.Errorf("delete alert %q: %w", ea.Name, err)
				}
			}
		}
	}

	return changes, nil
}

func applyMonitor(ctx context.Context, s store.Store, m Monitor, alertMap map[string]int, existing map[string]models.Site, parentID int, dryRun bool) ([]Change, error) {
	alertID, err := resolveAlertID(alertMap, m.Alert)
	if err != nil {
		return nil, fmt.Errorf("monitor %q: %w", m.Name, err)
	}
	site := monitorToSite(m, alertID, parentID)

	var changes []Change
	ex, exists := existing[m.Name]
	if !exists {
		changes = append(changes, Change{Action: "create", Kind: "monitor", Name: m.Name, Details: m.Type})
		if !dryRun {
			if _, err := s.AddSiteReturningID(ctx, site); err != nil {
				return changes, fmt.Errorf("create monitor %q: %w", m.Name, err)
			}
		}
	} else {
		site.ID = ex.ID
		if diff := diffSite(normalizeSite(ex), site); diff != "" {
			changes = append(changes, Change{Action: "update", Kind: "monitor", Name: m.Name, Details: diff})
			if !dryRun {
				if err := s.UpdateSite(ctx, site); err != nil {
					return changes, fmt.Errorf("update monitor %q: %w", m.Name, err)
				}
			}
		}
	}
	return changes, nil
}

func resolveAlertID(alertMap map[string]int, name string) (int, error) {
	if name == "" {
		return 0, nil
	}
	id, ok := alertMap[name]
	if !ok {
		return 0, fmt.Errorf("alert %q not found", name)
	}
	return id, nil
}

func monitorToSite(m Monitor, alertID, parentID int) models.Site {
	s := models.Site{
		Name:     m.Name,
		Type:     m.Type,
		URL:      m.URL,
		Interval: m.Interval,
		AlertID:  alertID,
		ParentID: parentID,

		CheckSSL:       m.CheckSSL,
		MaxRetries:     m.MaxRetries,
		Hostname:       m.Hostname,
		Port:           m.Port,
		Timeout:        m.Timeout,
		Description:    m.Description,
		DNSResolveType: m.DNSResolveType,
		DNSServer:      m.DNSServer,
		IgnoreTLS:      m.IgnoreTLS,
		Paused:         m.Paused,
		Regions:        m.Regions,
	}

	s.ExpiryThreshold = m.ExpiryThreshold
	if s.ExpiryThreshold == 0 {
		s.ExpiryThreshold = 7
	}

	s.Method = m.Method
	if s.Method == "" {
		s.Method = "GET"
	}

	s.AcceptedCodes = m.AcceptedCodes
	if s.AcceptedCodes == "" {
		s.AcceptedCodes = "200-299"
	}

	return s
}

func collectMonitorNames(monitors []Monitor, names map[string]bool) {
	for _, m := range monitors {
		names[m.Name] = true
		collectMonitorNames(m.Monitors, names)
	}
}

func normalizeSite(s models.Site) models.Site {
	if s.Method == "" {
		s.Method = "GET"
	}
	if s.AcceptedCodes == "" {
		s.AcceptedCodes = "200-299"
	}
	if s.ExpiryThreshold == 0 {
		s.ExpiryThreshold = 7
	}
	return s
}

func diffAlert(existing models.AlertConfig, desired Alert) string {
	var diffs []string
	if existing.Type != desired.Type {
		diffs = append(diffs, fmt.Sprintf("type: %s -> %s", existing.Type, desired.Type))
	}
	if !reflect.DeepEqual(existing.Settings, desired.Settings) {
		diffs = append(diffs, "settings changed")
	}
	return strings.Join(diffs, ", ")
}

func diffSite(existing, desired models.Site) string {
	var diffs []string
	if existing.URL != desired.URL {
		diffs = append(diffs, fmt.Sprintf("url: %s -> %s", existing.URL, desired.URL))
	}
	if existing.Type != desired.Type {
		diffs = append(diffs, fmt.Sprintf("type: %s -> %s", existing.Type, desired.Type))
	}
	if existing.Interval != desired.Interval {
		diffs = append(diffs, fmt.Sprintf("interval: %d -> %d", existing.Interval, desired.Interval))
	}
	if existing.AlertID != desired.AlertID {
		diffs = append(diffs, fmt.Sprintf("alert_id: %d -> %d", existing.AlertID, desired.AlertID))
	}
	if existing.CheckSSL != desired.CheckSSL {
		diffs = append(diffs, fmt.Sprintf("check_ssl: %v -> %v", existing.CheckSSL, desired.CheckSSL))
	}
	if existing.ExpiryThreshold != desired.ExpiryThreshold {
		diffs = append(diffs, fmt.Sprintf("expiry_threshold: %d -> %d", existing.ExpiryThreshold, desired.ExpiryThreshold))
	}
	if existing.MaxRetries != desired.MaxRetries {
		diffs = append(diffs, fmt.Sprintf("max_retries: %d -> %d", existing.MaxRetries, desired.MaxRetries))
	}
	if existing.Hostname != desired.Hostname {
		diffs = append(diffs, fmt.Sprintf("hostname: %s -> %s", existing.Hostname, desired.Hostname))
	}
	if existing.Port != desired.Port {
		diffs = append(diffs, fmt.Sprintf("port: %d -> %d", existing.Port, desired.Port))
	}
	if existing.Timeout != desired.Timeout {
		diffs = append(diffs, fmt.Sprintf("timeout: %d -> %d", existing.Timeout, desired.Timeout))
	}
	if existing.Method != desired.Method {
		diffs = append(diffs, fmt.Sprintf("method: %s -> %s", existing.Method, desired.Method))
	}
	if existing.Description != desired.Description {
		diffs = append(diffs, "description changed")
	}
	if existing.ParentID != desired.ParentID {
		diffs = append(diffs, fmt.Sprintf("parent_id: %d -> %d", existing.ParentID, desired.ParentID))
	}
	if existing.AcceptedCodes != desired.AcceptedCodes {
		diffs = append(diffs, fmt.Sprintf("accepted_codes: %s -> %s", existing.AcceptedCodes, desired.AcceptedCodes))
	}
	if existing.DNSResolveType != desired.DNSResolveType {
		diffs = append(diffs, fmt.Sprintf("dns_resolve_type: %s -> %s", existing.DNSResolveType, desired.DNSResolveType))
	}
	if existing.DNSServer != desired.DNSServer {
		diffs = append(diffs, fmt.Sprintf("dns_server: %s -> %s", existing.DNSServer, desired.DNSServer))
	}
	if existing.IgnoreTLS != desired.IgnoreTLS {
		diffs = append(diffs, fmt.Sprintf("ignore_tls: %v -> %v", existing.IgnoreTLS, desired.IgnoreTLS))
	}
	if existing.Paused != desired.Paused {
		diffs = append(diffs, fmt.Sprintf("paused: %v -> %v", existing.Paused, desired.Paused))
	}
	if existing.Regions != desired.Regions {
		diffs = append(diffs, fmt.Sprintf("regions: %s -> %s", existing.Regions, desired.Regions))
	}
	return strings.Join(diffs, ", ")
}

func FormatChanges(changes []Change, dryRun bool) string {
	var b strings.Builder
	if dryRun {
		b.WriteString("Dry run — no changes applied.\n\n")
	}

	if len(changes) == 0 {
		b.WriteString("No changes needed. State is up to date.\n")
		return b.String()
	}

	creates, updates, deletes := 0, 0, 0
	for _, c := range changes {
		var prefix string
		switch c.Action {
		case "create":
			prefix = "  + create"
			creates++
		case "update":
			prefix = "  ~ update"
			updates++
		case "delete":
			prefix = "  - delete"
			deletes++
		}
		line := fmt.Sprintf("%s %s %q", prefix, c.Kind, c.Name)
		if c.Details != "" {
			line += " (" + c.Details + ")"
		}
		b.WriteString(line + "\n")
	}

	b.WriteString("\n")
	if dryRun {
		fmt.Fprintf(&b, "Summary: %d to create, %d to update, %d to delete\n", creates, updates, deletes)
	} else {
		total := creates + updates + deletes
		fmt.Fprintf(&b, "Applied %d changes (%d created, %d updated, %d deleted)\n", total, creates, updates, deletes)
	}
	return b.String()
}
