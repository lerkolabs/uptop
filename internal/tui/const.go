package tui

import "time"

const (
	nodeOnlineThreshold = 60 * time.Second
	nodeStaleThreshold  = 5 * time.Minute

	uptimeGoodPct      = 99.0
	uptimeExcellentPct = 99.9
	uptimeWarnPct      = 95.0
	uptimePrecisionPct = 99.99

	stateHistoryLookback = 30 * 24 * time.Hour
	stateHistoryDays     = 30
	stateHistoryLimit    = 100

	httpErrorThreshold = 400
	errorDetailMaxLen  = 30
)
