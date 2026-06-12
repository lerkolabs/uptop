package cluster

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/monitor"
)

type Config struct {
	Mode      string // "leader" or "follower"
	PeerURL   string // URL of the Leader (e.g., http://primary:8080)
	SharedKey string // Security Key
}

func Start(ctx context.Context, cfg Config, eng *monitor.Engine) {
	if cfg.Mode == "leader" {
		fmt.Println("Cluster: Running as LEADER (Active)")
		if cfg.SharedKey != "" {
			fmt.Println("WARNING: Cluster mode enabled. Ensure the HTTP server is behind a TLS-terminating proxy.")
		}
		eng.SetActive(true)
		return
	}

	if cfg.Mode == "follower" {
		fmt.Println("Cluster: Running as FOLLOWER (Passive)")
		if cfg.PeerURL != "" && !strings.HasPrefix(cfg.PeerURL, "https://") {
			fmt.Println("WARNING: Cluster peer URL is not HTTPS. Cluster secret will be sent in cleartext.")
		}
		eng.SetActive(false)
		go runFollowerLoop(ctx, cfg, eng)
	}

	// "probe" mode is handled directly in main.go before cluster.Start is called
}

func runFollowerLoop(ctx context.Context, cfg Config, eng *monitor.Engine) {
	client := http.Client{Timeout: 2 * time.Second}
	failures := 0
	threshold := 3

	for {
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return
		}

		req, _ := http.NewRequest("GET", cfg.PeerURL+"/api/health", nil)
		if cfg.SharedKey != "" {
			req.Header.Set("X-Uptop-Secret", cfg.SharedKey)
		}

		resp, err := client.Do(req)
		isLeaderHealthy := false

		if err == nil {
			isLeaderHealthy = resp.StatusCode == 200
			_ = resp.Body.Close()
		}

		if isLeaderHealthy {
			failures = 0
			if eng.IsActive() {
				eng.SetActive(false)
				eng.AddLog("Cluster: Leader detected. Switching to PASSIVE.")
			}
		} else {
			failures++
			if failures >= threshold && !eng.IsActive() {
				eng.SetActive(true)
				eng.AddLog("Cluster: Leader Unreachable. Switching to ACTIVE.")
			}
		}
	}
}
