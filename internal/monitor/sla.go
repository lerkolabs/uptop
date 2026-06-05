package monitor

import (
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

type SLAReport struct {
	Window      time.Duration
	UptimePct   float64
	Downtime    time.Duration
	OutageCount int
	LongestOut  time.Duration
	MTTR        time.Duration
	MTBF        time.Duration
}

func ComputeSLA(changes []models.StateChange, currentStatus string, window time.Duration) SLAReport {
	now := time.Now()
	windowStart := now.Add(-window)

	report := SLAReport{Window: window}

	if len(changes) == 0 {
		if isDown(currentStatus) {
			report.UptimePct = 0
			report.Downtime = window
		} else {
			report.UptimePct = 100
		}
		return report
	}

	// Sort changes chronologically (they come in DESC from DB).
	sorted := make([]models.StateChange, len(changes))
	copy(sorted, changes)
	for i, j := 0, len(sorted)-1; i < j; i, j = i+1, j-1 {
		sorted[i], sorted[j] = sorted[j], sorted[i]
	}

	// Determine status at window start: last transition before or at windowStart.
	statusAtStart := "UP"
	for i := len(sorted) - 1; i >= 0; i-- {
		if !sorted[i].ChangedAt.After(windowStart) {
			statusAtStart = sorted[i].ToStatus
			break
		}
	}

	var upTime, downTime time.Duration
	var outages []time.Duration
	cursor := windowStart
	wasDown := isDown(statusAtStart)

	if wasDown {
		report.OutageCount = 1
	}

	var outageStart time.Time
	if wasDown {
		outageStart = windowStart
	}

	for _, sc := range sorted {
		if sc.ChangedAt.Before(windowStart) {
			continue
		}
		if sc.ChangedAt.After(now) {
			break
		}

		seg := sc.ChangedAt.Sub(cursor)
		if wasDown {
			downTime += seg
		} else {
			upTime += seg
		}

		newDown := isDown(sc.ToStatus)
		if !wasDown && newDown {
			report.OutageCount++
			outageStart = sc.ChangedAt
		}
		if wasDown && !newDown {
			dur := sc.ChangedAt.Sub(outageStart)
			outages = append(outages, dur)
		}

		wasDown = newDown
		cursor = sc.ChangedAt
	}

	// Account for time from last change to now.
	remaining := now.Sub(cursor)
	if wasDown {
		downTime += remaining
		dur := now.Sub(outageStart)
		outages = append(outages, dur)
	} else {
		upTime += remaining
	}

	total := upTime + downTime
	if total > 0 {
		report.UptimePct = float64(upTime) / float64(total) * 100
	} else {
		report.UptimePct = 100
	}
	report.Downtime = downTime

	if len(outages) > 0 {
		var totalOutage time.Duration
		for _, d := range outages {
			totalOutage += d
			if d > report.LongestOut {
				report.LongestOut = d
			}
		}
		report.MTTR = totalOutage / time.Duration(len(outages))
	}

	if report.OutageCount > 0 && upTime > 0 {
		report.MTBF = upTime / time.Duration(report.OutageCount)
	}

	return report
}

func ComputeDailyBreakdown(changes []models.StateChange, currentStatus string, days int, now time.Time) []DayReport {
	reports := make([]DayReport, days)

	for i := 0; i < days; i++ {
		dayEnd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(-time.Duration(i) * 24 * time.Hour)
		if i == 0 {
			dayEnd = now
		}
		dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(-time.Duration(i) * 24 * time.Hour)
		if i > 0 {
			dayEnd = dayStart.Add(24 * time.Hour)
		}

		windowChanges := filterChangesForWindow(changes, dayStart, dayEnd)

		statusAtStart := inferStatusAt(changes, dayStart)
		sla := computeSLAForWindow(windowChanges, statusAtStart, dayStart, dayEnd)

		reports[i] = DayReport{
			Date:      dayStart,
			UptimePct: sla,
		}
	}

	return reports
}

type DayReport struct {
	Date      time.Time
	UptimePct float64
}

func isDown(status string) bool {
	return status == "DOWN" || status == "SSL EXP"
}

func filterChangesForWindow(changes []models.StateChange, start, end time.Time) []models.StateChange {
	var filtered []models.StateChange
	for _, sc := range changes {
		if !sc.ChangedAt.Before(start) && sc.ChangedAt.Before(end) {
			filtered = append(filtered, sc)
		}
	}
	return filtered
}

func inferStatusAt(changes []models.StateChange, at time.Time) string {
	// Changes come DESC from DB. Walk backwards to find last change before `at`.
	for _, sc := range changes {
		if !sc.ChangedAt.After(at) {
			return sc.ToStatus
		}
	}
	return "UP"
}

func computeSLAForWindow(changes []models.StateChange, statusAtStart string, start, end time.Time) float64 {
	// Sort chronologically.
	sorted := make([]models.StateChange, len(changes))
	copy(sorted, changes)
	for i, j := 0, len(sorted)-1; i < j; i, j = i+1, j-1 {
		sorted[i], sorted[j] = sorted[j], sorted[i]
	}

	var upTime, downTime time.Duration
	cursor := start
	wasDown := isDown(statusAtStart)

	for _, sc := range sorted {
		if sc.ChangedAt.Before(start) || !sc.ChangedAt.Before(end) {
			continue
		}
		seg := sc.ChangedAt.Sub(cursor)
		if wasDown {
			downTime += seg
		} else {
			upTime += seg
		}
		wasDown = isDown(sc.ToStatus)
		cursor = sc.ChangedAt
	}

	remaining := end.Sub(cursor)
	if wasDown {
		downTime += remaining
	} else {
		upTime += remaining
	}

	total := upTime + downTime
	if total <= 0 {
		return 100
	}
	return float64(upTime) / float64(total) * 100
}
