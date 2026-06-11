package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/monitor"
)

func Handler(eng *monitor.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sites := eng.GetAllSites()
		sort.Slice(sites, func(i, j int) bool { return sites[i].ID < sites[j].ID })

		var b strings.Builder

		writeHelp(&b, "uptop_monitor_up", "gauge", "Whether the monitor is up (1) or down (0).")
		for _, s := range sites {
			val := 0
			if s.Status == models.StatusUp {
				val = 1
			}
			writeGauge(&b, "uptop_monitor_up", labels(s), float64(val))
		}

		writeHelp(&b, "uptop_monitor_latency_seconds", "gauge", "Last check latency in seconds.")
		for _, s := range sites {
			writeGauge(&b, "uptop_monitor_latency_seconds", labels(s), s.Latency.Seconds())
		}

		writeHelp(&b, "uptop_monitor_status_code", "gauge", "HTTP response status code of the last check.")
		for _, s := range sites {
			if s.Type != "http" {
				continue
			}
			writeGauge(&b, "uptop_monitor_status_code", labels(s), float64(s.StatusCode))
		}

		writeHelp(&b, "uptop_monitor_check_timestamp_seconds", "gauge", "Unix timestamp of the last check.")
		for _, s := range sites {
			if s.LastCheck.IsZero() {
				continue
			}
			writeGauge(&b, "uptop_monitor_check_timestamp_seconds", labels(s), float64(s.LastCheck.Unix()))
		}

		writeHelp(&b, "uptop_monitor_paused", "gauge", "Whether the monitor is paused (1) or active (0).")
		for _, s := range sites {
			val := 0
			if s.Paused {
				val = 1
			}
			writeGauge(&b, "uptop_monitor_paused", labels(s), float64(val))
		}

		writeHelp(&b, "uptop_monitor_maintenance", "gauge", "Whether the monitor is in a maintenance window (1) or not (0).")
		for _, s := range sites {
			val := 0
			if eng.GetDisplayStatus(s) == "MAINT" {
				val = 1
			}
			writeGauge(&b, "uptop_monitor_maintenance", labels(s), float64(val))
		}

		writeHelp(&b, "uptop_monitor_cert_expiry_timestamp_seconds", "gauge", "Unix timestamp when the SSL certificate expires.")
		for _, s := range sites {
			if !s.HasSSL || s.CertExpiry.IsZero() {
				continue
			}
			writeGauge(&b, "uptop_monitor_cert_expiry_timestamp_seconds", labels(s), float64(s.CertExpiry.Unix()))
		}

		writeHelp(&b, "uptop_monitor_checks_total", "counter", "Total number of checks performed.")
		writeHelp(&b, "uptop_monitor_checks_up_total", "counter", "Total number of successful checks.")
		for _, s := range sites {
			h, ok := eng.GetHistory(s.ID)
			if !ok {
				continue
			}
			writeGauge(&b, "uptop_monitor_checks_total", labels(s), float64(h.TotalChecks))
			writeGauge(&b, "uptop_monitor_checks_up_total", labels(s), float64(h.UpChecks))
		}

		writeHelp(&b, "uptop_probe_up", "gauge", "Whether a probe node is online (1) or offline (0) based on last-seen time.")
		for _, site := range sites {
			probeResults := eng.GetProbeResults(site.ID)
			for nodeID, result := range probeResults {
				val := 0
				if result.IsUp {
					val = 1
				}
				nodeLabels := fmt.Sprintf(`id="%d",name="%s",node="%s"`, site.ID, escapeLabelValue(site.Name), escapeLabelValue(nodeID))
				writeGauge(&b, "uptop_probe_up", nodeLabels, float64(val))
			}
		}

		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(b.String())) //nolint:errcheck
	}
}

func labels(s models.Site) string {
	return fmt.Sprintf(`id="%d",name="%s",type="%s"`, s.ID, escapeLabelValue(s.Name), s.Type)
}

func escapeLabelValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

func writeHelp(b *strings.Builder, name, typ, help string) {
	fmt.Fprintf(b, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, typ)
}

func writeGauge(b *strings.Builder, name, labels string, val float64) {
	fmt.Fprintf(b, "%s{%s} %g\n", name, labels, val)
}
