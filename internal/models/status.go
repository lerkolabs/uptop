package models

type Status string

const (
	StatusUp      Status = "UP"
	StatusDown    Status = "DOWN"
	StatusPending Status = "PENDING"
	StatusLate    Status = "LATE"
	StatusStale   Status = "STALE"
	StatusSSLExp  Status = "SSL EXP"
)

func (s Status) IsBroken() bool {
	return s == StatusDown || s == StatusSSLExp
}

func (s Status) String() string { return string(s) }
