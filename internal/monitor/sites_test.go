package monitor

import (
	"testing"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

func TestUpdateSiteConfig_PreservesRuntime(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "test", URL: "http://old.com"},
		SiteState:  models.SiteState{Status: "DOWN", FailureCount: 3, Latency: 100 * time.Millisecond},
	}
	injectSite(e, site)

	updated := models.SiteConfig{ID: 1, Name: "test", URL: "http://new.com", Interval: 60}
	e.UpdateSiteConfig(updated)

	s, _ := getSite(e, 1)
	if s.URL != "http://new.com" {
		t.Errorf("expected URL updated, got %s", s.URL)
	}
	if s.Status != "DOWN" {
		t.Errorf("expected Status preserved, got %s", s.Status)
	}
	if s.FailureCount != 3 {
		t.Errorf("expected FailureCount preserved, got %d", s.FailureCount)
	}
	if s.Latency != 100*time.Millisecond {
		t.Errorf("expected Latency preserved, got %v", s.Latency)
	}
}

func TestRemoveSite_CleansUp(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "test", Type: "push", Token: "tok1"},
		SiteState:  models.SiteState{Status: "UP"},
	}
	injectSite(e, site)
	e.recordCheck(1, 5*time.Millisecond, true)

	e.RemoveSite(1)

	if _, ok := getSite(e, 1); ok {
		t.Error("expected site removed from liveState")
	}
	if e.RecordHeartbeat("tok1") {
		t.Error("expected token removed from index")
	}
	if _, ok := e.GetHistory(1); ok {
		t.Error("expected history removed")
	}
}

func TestToggleSitePause(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "test"},
		SiteState:  models.SiteState{Status: "UP"},
	}
	injectSite(e, site)

	paused := e.ToggleSitePause(1)
	if !paused {
		t.Error("expected paused=true after first toggle")
	}
	s, _ := getSite(e, 1)
	if !s.Paused {
		t.Error("expected Paused=true in state")
	}

	paused = e.ToggleSitePause(1)
	if paused {
		t.Error("expected paused=false after second toggle")
	}
}

func TestToggleSitePause_NonexistentSite(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	if e.ToggleSitePause(999) {
		t.Error("expected false for nonexistent site")
	}
}

func TestGetAllSites_ReturnsCopy(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	injectSite(e, models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "s1"},
		SiteState:  models.SiteState{Status: "UP"},
	})
	injectSite(e, models.Site{
		SiteConfig: models.SiteConfig{ID: 2, Name: "s2"},
		SiteState:  models.SiteState{Status: "DOWN"},
	})

	sites := e.GetAllSites()
	if len(sites) != 2 {
		t.Fatalf("expected 2 sites, got %d", len(sites))
	}
	sites[0].Name = "mutated"

	fresh := e.GetAllSites()
	for _, s := range fresh {
		if s.Name == "mutated" {
			t.Error("GetAllSites returned reference, not copy")
		}
	}
}

func TestGetLiveState_ReturnsCopy(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	injectSite(e, models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "s1"},
		SiteState:  models.SiteState{Status: "UP"},
	})

	state := e.GetLiveState()
	state[1] = models.Site{SiteConfig: models.SiteConfig{Name: "mutated"}}

	fresh := e.GetLiveState()
	if fresh[1].Name == "mutated" {
		t.Error("GetLiveState returned reference, not copy")
	}
}
