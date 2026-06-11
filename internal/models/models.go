package models

import "time"

type SiteConfig struct {
	ID              int
	Name            string
	URL             string
	Type            string // "http", "push", "ping", "port", "dns", "group"
	Token           string
	Interval        int
	AlertID         int
	CheckSSL        bool
	ExpiryThreshold int
	MaxRetries      int

	Hostname       string
	Port           int
	Timeout        int
	Method         string
	Description    string
	ParentID       int
	AcceptedCodes  string
	DNSResolveType string
	DNSServer      string
	IgnoreTLS      bool
	Paused         bool
	Regions        string
}

type SiteState struct {
	FailureCount    int
	Status          Status
	StatusCode      int
	Latency         time.Duration
	CertExpiry      time.Time
	HasSSL          bool
	LastCheck       time.Time
	SentSSLWarning  bool
	LastError       string
	StatusChangedAt time.Time
	LastSuccessAt   time.Time
}

type Site struct {
	SiteConfig
	SiteState
}

type StateChange struct {
	ID          int
	SiteID      int
	FromStatus  string
	ToStatus    string
	ErrorReason string
	ChangedAt   time.Time
}

type AlertConfig struct {
	ID       int
	Name     string
	Type     string
	Settings map[string]string
}

type User struct {
	ID        int
	Username  string
	PublicKey string
	Role      string
}

type CheckRecord struct {
	SiteID    int
	NodeID    string
	LatencyNs int64
	IsUp      bool
	CheckedAt time.Time
}

type ProbeNode struct {
	ID       string
	Name     string
	Region   string
	LastSeen time.Time
	Version  string
}

// AlertHealthRecord is the persisted send health of an alert channel. It lets the
// "last sent" / health indicators survive restarts instead of resetting to "never".
type AlertHealthRecord struct {
	AlertID    int
	LastSendAt time.Time
	LastSendOK bool
	LastError  string
	SendCount  int
	FailCount  int
}

type MaintenanceWindow struct {
	ID          int
	MonitorID   int
	Title       string
	Description string
	Type        string // "maintenance" or "incident"
	StartTime   time.Time
	EndTime     time.Time // zero = ongoing
	CreatedBy   string
	CreatedAt   time.Time
}

type Backup struct {
	Sites              []SiteConfig        `json:"sites"`
	Alerts             []AlertConfig       `json:"alerts"`
	Users              []User              `json:"users"`
	MaintenanceWindows []MaintenanceWindow `json:"maintenance_windows,omitempty"`
}
