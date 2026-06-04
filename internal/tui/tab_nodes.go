package tui

import (
	"fmt"
	"time"
)

func (m Model) viewNodesTab() string {
	if len(m.nodes) == 0 {
		return m.emptyState("No probe nodes connected.", "")
	}

	var headers []string
	var widths []int
	if m.isWide() {
		headers = []string{"NAME", "REGION", "LAST SEEN", "VERSION", "STATUS"}
		widths = []int{24, 14, 16, 12, 10}
	} else {
		headers = []string{"NAME", "REGION", "SEEN", "VER", "STATUS"}
		widths = []int{16, 10, 10, 8, 8}
	}
	nameW := widths[0]

	return m.renderTable(
		headers,
		len(m.nodes),
		func(start, end int) [][]string {
			var rows [][]string
			for i := start; i < end; i++ {
				node := m.nodes[i]
				name := limitStr(node.Name, nameW-2)
				if name == "" {
					name = node.ID
				}
				region := node.Region
				if region == "" {
					region = subtleStyle.Render("—")
				}
				lastSeen := fmtNodeLastSeen(node.LastSeen)
				version := node.Version
				if version == "" {
					version = subtleStyle.Render("—")
				}
				status := fmtNodeStatus(node.LastSeen)
				rows = append(rows, []string{name, region, lastSeen, version, status})
			}
			return rows
		},
		widths,
		nil,
	)
}

func fmtNodeStatus(lastSeen time.Time) string {
	if lastSeen.IsZero() {
		return subtleStyle.Render("UNKNOWN")
	}
	ago := time.Since(lastSeen)
	if ago < 60*time.Second {
		return specialStyle.Render("ONLINE")
	}
	if ago < 5*time.Minute {
		return warnStyle.Render("STALE")
	}
	return dangerStyle.Render("OFFLINE")
}

func fmtNodeLastSeen(t time.Time) string {
	if t.IsZero() {
		return subtleStyle.Render("never")
	}
	ago := time.Since(t)
	if ago < time.Minute {
		return fmt.Sprintf("%ds ago", int(ago.Seconds()))
	}
	if ago < time.Hour {
		return fmt.Sprintf("%dm ago", int(ago.Minutes()))
	}
	return fmt.Sprintf("%dh ago", int(ago.Hours()))
}
