package cluster

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/monitor"
)

type ProbeConfig struct {
	NodeID              string
	NodeName            string
	Region              string
	LeaderURL           string
	SharedKey           string
	Interval            int
	AllowPrivateTargets bool
}

func RunProbe(ctx context.Context, cfg ProbeConfig) error {
	if cfg.Interval < 10 {
		cfg.Interval = 30
	}

	apiClient := &http.Client{Timeout: 10 * time.Second}
	dial := monitor.SafeDialContext(cfg.AllowPrivateTargets)
	strictClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
			DialContext:     dial,
		},
	}
	insecureClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // intentional for IgnoreTLS sites
			DialContext:     dial,
		},
	}

	if err := probeRegister(ctx, apiClient, cfg); err != nil {
		slog.Error("probe initial registration failed", "err", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		sites, err := probeFetchAssignments(ctx, apiClient, cfg)
		if err != nil {
			slog.Error("probe failed to fetch assignments", "err", err)
			sleepCtx(ctx, 10*time.Second)
			continue
		}

		if len(sites) == 0 {
			sleepCtx(ctx, time.Duration(cfg.Interval)*time.Second)
			continue
		}

		results := probeExecuteChecks(ctx, sites, strictClient, insecureClient, cfg.AllowPrivateTargets)

		if len(results) > 0 {
			if err := probeReportResults(ctx, apiClient, cfg, results); err != nil {
				slog.Error("probe failed to report results", "err", err)
			}
		}

		sleepCtx(ctx, time.Duration(cfg.Interval)*time.Second)
	}
}

func probeRegister(ctx context.Context, client *http.Client, cfg ProbeConfig) error {
	body, _ := json.Marshal(map[string]string{
		"id": cfg.NodeID, "name": cfg.NodeName, "region": cfg.Region, "version": "probe",
	})
	req, err := http.NewRequestWithContext(ctx, "POST", cfg.LeaderURL+"/api/probe/register", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Upkeep-Secret", cfg.SharedKey)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("register returned %d", resp.StatusCode)
	}
	return nil
}

func probeFetchAssignments(ctx context.Context, client *http.Client, cfg ProbeConfig) ([]models.Site, error) {
	assignURL := cfg.LeaderURL + "/api/probe/assignments?" + url.Values{"node_id": {cfg.NodeID}}.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", assignURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Upkeep-Secret", cfg.SharedKey)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("assignments returned %d", resp.StatusCode)
	}
	var result struct {
		Sites []models.Site `json:"sites"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Sites, nil
}

type probeResultItem struct {
	SiteID      int    `json:"site_id"`
	LatencyNs   int64  `json:"latency_ns"`
	IsUp        bool   `json:"is_up"`
	ErrorReason string `json:"error_reason,omitempty"`
}

func probeExecuteChecks(ctx context.Context, sites []models.Site, strict, insecure *http.Client, allowPrivate bool) []probeResultItem {
	var mu sync.Mutex
	var results []probeResultItem
	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup

loop:
	for _, site := range sites {
		select {
		case <-ctx.Done():
			break loop
		default:
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(s models.Site) {
			defer wg.Done()
			defer func() { <-sem }()

			cr := monitor.RunCheck(ctx, s.SiteConfig, strict, insecure, false, allowPrivate)
			mu.Lock()
			results = append(results, probeResultItem{
				SiteID:      s.ID,
				LatencyNs:   cr.LatencyNs,
				IsUp:        cr.Status == string(models.StatusUp),
				ErrorReason: cr.ErrorReason,
			})
			mu.Unlock()
		}(site)
	}
	wg.Wait()
	return results
}

func probeReportResults(ctx context.Context, client *http.Client, cfg ProbeConfig, results []probeResultItem) error {
	body, err := json.Marshal(map[string]interface{}{
		"node_id": cfg.NodeID,
		"results": results,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", cfg.LeaderURL+"/api/probe/results", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Upkeep-Secret", cfg.SharedKey)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("results returned %d", resp.StatusCode)
	}
	slog.Info("probe reported check results", "count", len(results))
	return nil
}

func sleepCtx(ctx context.Context, d time.Duration) {
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}
