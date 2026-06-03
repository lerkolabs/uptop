package tui

import (
	"testing"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

func TestSortSitesForDisplay_GroupsFirst(t *testing.T) {
	sites := []models.Site{
		{ID: 3, Name: "ungrouped", Type: "http", Status: "UP"},
		{ID: 1, Name: "group-a", Type: "group", Status: "UP"},
		{ID: 2, Name: "child", Type: "http", Status: "UP", ParentID: 1},
	}
	result := sortSitesForDisplay(sites, nil)
	if len(result) != 3 {
		t.Fatalf("expected 3 sites, got %d", len(result))
	}
	if result[0].Name != "group-a" {
		t.Errorf("first should be group, got %s", result[0].Name)
	}
	if result[1].Name != "child" {
		t.Errorf("second should be child, got %s", result[1].Name)
	}
	if result[2].Name != "ungrouped" {
		t.Errorf("third should be ungrouped, got %s", result[2].Name)
	}
}

func TestSortSitesForDisplay_CollapsedHidesChildren(t *testing.T) {
	sites := []models.Site{
		{ID: 1, Name: "group-a", Type: "group", Status: "UP"},
		{ID: 2, Name: "child-1", Type: "http", Status: "UP", ParentID: 1},
		{ID: 3, Name: "child-2", Type: "http", Status: "UP", ParentID: 1},
	}
	collapsed := map[int]bool{1: true}
	result := sortSitesForDisplay(sites, collapsed)
	if len(result) != 1 {
		t.Fatalf("collapsed group should hide children, got %d items", len(result))
	}
	if result[0].Name != "group-a" {
		t.Errorf("only group should remain, got %s", result[0].Name)
	}
}

func TestSortSitesForDisplay_StatusOrdering(t *testing.T) {
	sites := []models.Site{
		{ID: 1, Name: "up-site", Type: "http", Status: "UP"},
		{ID: 2, Name: "down-site", Type: "http", Status: "DOWN"},
		{ID: 3, Name: "late-site", Type: "http", Status: "LATE"},
	}
	result := sortSitesForDisplay(sites, nil)
	if result[0].Status != "DOWN" {
		t.Errorf("DOWN should sort first, got %s", result[0].Status)
	}
	if result[1].Status != "LATE" {
		t.Errorf("LATE should sort second, got %s", result[1].Status)
	}
	if result[2].Status != "UP" {
		t.Errorf("UP should sort third, got %s", result[2].Status)
	}
}

func TestFilterSites(t *testing.T) {
	sites := []models.Site{
		{Name: "Production API"},
		{Name: "Staging API"},
		{Name: "Database"},
	}

	tests := []struct {
		needle string
		want   int
	}{
		{"api", 2},
		{"API", 2},
		{"database", 1},
		{"nonexistent", 0},
		{"", 3},
	}
	for _, tt := range tests {
		got := filterSites(sites, tt.needle)
		if len(got) != tt.want {
			t.Errorf("filterSites(%q) returned %d, want %d", tt.needle, len(got), tt.want)
		}
	}
}

func TestFilterSites_EmptyNeedle(t *testing.T) {
	sites := []models.Site{{Name: "a"}, {Name: "b"}}
	got := filterSites(sites, "")
	if len(got) != 2 {
		t.Errorf("empty needle should return all, got %d", len(got))
	}
}
