package monitor

import (
	"context"
	"testing"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

// Maintenance cache resolves parent relationships correctly.
func TestIsInMaintenance_UsesCache(t *testing.T) {
	ms := newMockStore()
	ms.maintenance[10] = true // direct maintenance on group
	e := newTestEngine(ms)
	group := models.Site{
		SiteConfig: models.SiteConfig{ID: 10, Name: "group", Type: "group"},
		SiteState:  models.SiteState{Status: "UP"},
	}
	child := models.Site{
		SiteConfig: models.SiteConfig{ID: 20, Name: "child", Type: "http", ParentID: 10},
		SiteState:  models.SiteState{Status: "UP"},
	}
	injectSite(e, group)
	injectSite(e, child)
	e.refreshMaintenanceCache(context.Background())

	if !e.isInMaintenance(10) {
		t.Error("group should be in maintenance (direct)")
	}
	if !e.isInMaintenance(20) {
		t.Error("child should be in maintenance (parent)")
	}
	if e.isInMaintenance(99) {
		t.Error("unknown monitor should not be in maintenance")
	}
}

// Global maintenance (monitor_id=0) applies to all monitors.
func TestIsInMaintenance_GlobalMaintenance(t *testing.T) {
	ms := newMockStore()
	ms.maintenance[0] = true
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "test", Type: "http"},
		SiteState:  models.SiteState{Status: "UP"},
	}
	injectSite(e, site)
	e.refreshMaintenanceCache(context.Background())

	if !e.isInMaintenance(1) {
		t.Error("all monitors should be in maintenance during global window")
	}
}
