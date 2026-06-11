package config

import (
	"context"
	"fmt"
	"os"
	"sort"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/store"

	"gopkg.in/yaml.v3"
)

func Export(ctx context.Context, s store.Store) (*File, error) {
	dbAlerts, err := s.GetAllAlerts(ctx)
	if err != nil {
		return nil, fmt.Errorf("load alerts: %w", err)
	}

	dbSites, err := s.GetSites(ctx)
	if err != nil {
		return nil, fmt.Errorf("load sites: %w", err)
	}

	alertIDToName := make(map[int]string, len(dbAlerts))
	var yamlAlerts []Alert
	for _, a := range dbAlerts {
		alertIDToName[a.ID] = a.Name
		yamlAlerts = append(yamlAlerts, Alert{
			Name:     a.Name,
			Type:     a.Type,
			Settings: a.Settings,
		})
	}

	groups := make(map[int]models.SiteConfig)
	children := make(map[int][]models.SiteConfig)
	var topLevel []models.SiteConfig

	for _, s := range dbSites {
		switch {
		case s.Type == "group":
			groups[s.ID] = s
		case s.ParentID > 0:
			children[s.ParentID] = append(children[s.ParentID], s)
		default:
			topLevel = append(topLevel, s)
		}
	}

	var yamlMonitors []Monitor

	groupIDs := make([]int, 0, len(groups))
	for id := range groups {
		groupIDs = append(groupIDs, id)
	}
	sort.Ints(groupIDs)

	for _, gid := range groupIDs {
		g := groups[gid]
		ym := siteToMonitor(g, alertIDToName)
		kids := children[gid]
		sort.Slice(kids, func(i, j int) bool { return kids[i].ID < kids[j].ID })
		for _, child := range kids {
			ym.Monitors = append(ym.Monitors, siteToMonitor(child, alertIDToName))
		}
		yamlMonitors = append(yamlMonitors, ym)
	}

	sort.Slice(topLevel, func(i, j int) bool { return topLevel[i].ID < topLevel[j].ID })
	for _, s := range topLevel {
		yamlMonitors = append(yamlMonitors, siteToMonitor(s, alertIDToName))
	}

	return &File{Alerts: yamlAlerts, Monitors: yamlMonitors}, nil
}

func siteToMonitor(s models.SiteConfig, alertIDToName map[int]string) Monitor {
	m := Monitor{
		Name:     s.Name,
		Type:     s.Type,
		Interval: s.Interval,
	}

	if s.AlertID > 0 {
		if name, ok := alertIDToName[s.AlertID]; ok {
			m.Alert = name
		}
	}

	if s.URL != "" {
		m.URL = s.URL
	}
	if s.Hostname != "" {
		m.Hostname = s.Hostname
	}
	if s.Port != 0 {
		m.Port = s.Port
	}
	if s.Timeout != 0 {
		m.Timeout = s.Timeout
	}
	if s.Description != "" {
		m.Description = s.Description
	}
	if s.DNSResolveType != "" {
		m.DNSResolveType = s.DNSResolveType
	}
	if s.DNSServer != "" {
		m.DNSServer = s.DNSServer
	}

	if s.Method != "" && s.Method != "GET" {
		m.Method = s.Method
	}
	if s.AcceptedCodes != "" && s.AcceptedCodes != "200-299" {
		m.AcceptedCodes = s.AcceptedCodes
	}
	if s.ExpiryThreshold != 0 && s.ExpiryThreshold != 7 {
		m.ExpiryThreshold = s.ExpiryThreshold
	}
	if s.MaxRetries != 0 {
		m.MaxRetries = s.MaxRetries
	}

	m.CheckSSL = s.CheckSSL
	m.IgnoreTLS = s.IgnoreTLS
	m.Paused = s.Paused

	if s.Regions != "" {
		m.Regions = s.Regions
	}

	return m
}

func WriteFile(f *File, path string) error {
	data, err := yaml.Marshal(f)
	if err != nil {
		return fmt.Errorf("marshal yaml: %w", err)
	}
	if path == "-" || path == "" {
		_, err = os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func LoadFile(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &f, nil
}
