package main

import (
	"net"
	"os"
	"strconv"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/server"
)

type appConfig struct {
	Port       int
	SSHHostKey string

	DBType string
	DBDSN  string

	HTTPPort int
	TLSCert  string
	TLSKey   string

	StatusEnabled bool
	StatusTitle   string

	ClusterMode   string
	ClusterSecret string
	PeerURL       string
	NodeID        string
	NodeName      string
	NodeRegion    string

	AggStrategy         string
	AllowPrivateTargets bool
	InsecureSkipVerify  bool
	MaintRetention      time.Duration
	EncryptionKey       string

	MetricsPublic  bool
	CORSOrigin     string
	TrustedProxies []*net.IPNet

	AdminKey string
	KeysFile string
}

func parseConfig() appConfig {
	cfg := appConfig{
		Port:           23234,
		SSHHostKey:     ".ssh/id_ed25519",
		DBType:         "sqlite",
		DBDSN:          "uptop.db",
		HTTPPort:       8080,
		StatusTitle:    "System Status",
		ClusterMode:    "leader",
		MaintRetention: 7 * 24 * time.Hour,
	}

	if v := os.Getenv("UPTOP_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Port = n
		}
	}
	if v := os.Getenv("UPTOP_DB_TYPE"); v != "" {
		cfg.DBType = v
	}
	if v := os.Getenv("UPTOP_DB_DSN"); v != "" {
		cfg.DBDSN = v
	}
	if v := os.Getenv("UPTOP_HTTP_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.HTTPPort = n
		}
	}
	if os.Getenv("UPTOP_STATUS_ENABLED") == "true" {
		cfg.StatusEnabled = true
	}
	if v := os.Getenv("UPTOP_STATUS_TITLE"); v != "" {
		cfg.StatusTitle = v
	}
	if v := os.Getenv("UPTOP_CLUSTER_MODE"); v != "" {
		cfg.ClusterMode = v
	}
	if v := os.Getenv("UPTOP_PEER_URL"); v != "" {
		cfg.PeerURL = v
	}
	if v := os.Getenv("UPTOP_CLUSTER_SECRET"); v != "" {
		cfg.ClusterSecret = v
	}

	cfg.NodeID = os.Getenv("UPTOP_NODE_ID")
	cfg.NodeName = os.Getenv("UPTOP_NODE_NAME")
	cfg.NodeRegion = os.Getenv("UPTOP_NODE_REGION")
	cfg.AggStrategy = os.Getenv("UPTOP_AGG_STRATEGY")

	cfg.AllowPrivateTargets = os.Getenv("UPTOP_ALLOW_PRIVATE_TARGETS") == "true"
	cfg.InsecureSkipVerify = os.Getenv("UPTOP_INSECURE_SKIP_VERIFY") == "true"
	cfg.MetricsPublic = os.Getenv("UPTOP_METRICS_PUBLIC") == "true"

	cfg.EncryptionKey = os.Getenv("UPTOP_ENCRYPTION_KEY")
	cfg.TLSCert = os.Getenv("UPTOP_TLS_CERT")
	cfg.TLSKey = os.Getenv("UPTOP_TLS_KEY")
	cfg.CORSOrigin = os.Getenv("UPTOP_CORS_ORIGIN")
	cfg.TrustedProxies = parseTrustedProxies(os.Getenv("UPTOP_TRUSTED_PROXIES"))

	cfg.SSHHostKey = envOrDefault("UPTOP_SSH_HOST_KEY", cfg.SSHHostKey)
	cfg.AdminKey = os.Getenv("UPTOP_ADMIN_KEY")
	cfg.KeysFile = os.Getenv("UPTOP_KEYS")

	if v := os.Getenv("UPTOP_MAINT_RETENTION"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.MaintRetention = d
		}
	}

	return cfg
}

func (c appConfig) serverConfig(quietHTTPLog bool) server.ServerConfig {
	return server.ServerConfig{
		Port:           c.HTTPPort,
		EnableStatus:   c.StatusEnabled,
		Title:          c.StatusTitle,
		ClusterKey:     c.ClusterSecret,
		TLSCert:        c.TLSCert,
		TLSKey:         c.TLSKey,
		ClusterMode:    c.ClusterMode,
		MetricsPublic:  c.MetricsPublic,
		CORSOrigin:     c.CORSOrigin,
		TrustedProxies: c.TrustedProxies,
		QuietHTTPLog:   quietHTTPLog,
	}
}
