package tui

import (
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
					region = m.st.subtleStyle.Render("—")
				}
				lastSeen := m.fmtNodeLastSeen(node.LastSeen)
				version := node.Version
				if version == "" {
					version = m.st.subtleStyle.Render("—")
				}
				status := m.fmtNodeStatus(node.LastSeen)
				rows = append(rows, []string{name, region, lastSeen, version, status})
			}
			return rows
		},
		widths,
		nil,
	)
}

func (m Model) fmtNodeStatus(lastSeen time.Time) string {
	if lastSeen.IsZero() {
		return m.st.subtleStyle.Render("UNKNOWN")
	}
	ago := time.Since(lastSeen)
	if ago < 60*time.Second {
		return m.st.specialStyle.Render("ONLINE")
	}
	if ago < 5*time.Minute {
		return m.st.warnStyle.Render("STALE")
	}
	return m.st.dangerStyle.Render("OFFLINE")
}

func (m Model) fmtNodeLastSeen(t time.Time) string {
	return m.fmtTimeAgo(t)
}
