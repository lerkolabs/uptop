package monitor

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"

	"github.com/miekg/dns"
	probing "github.com/prometheus-community/pro-bing"
)

const (
	maxErrorLength       = 256
	defaultAcceptedCodes = "200-299"
	defaultHTTPStatusMin = 200
	defaultHTTPStatusMax = 300
	defaultTimeout       = 5 * time.Second
	defaultDNSServer     = "1.1.1.1"
	defaultDNSPort       = "53"
)

type CheckResult struct {
	SiteID      int
	Status      string // "UP", "DOWN", "SSL EXP"
	StatusCode  int
	LatencyNs   int64
	HasSSL      bool
	CertExpiry  time.Time
	ErrorReason string
}

func RunCheck(ctx context.Context, site models.SiteConfig, strict, insecure *http.Client, globalInsecure, allowPrivate bool) CheckResult {
	// Resolve + validate once for non-HTTP types to prevent DNS-rebind TOCTOU:
	// a second resolve in the check function could return a different (private) IP.
	// HTTP is safe — SafeDialContext resolves and validates at dial time.
	var pinnedIP net.IP
	if site.Type != "http" && site.Type != "dns" && !allowPrivate {
		host := site.Hostname
		if host == "" {
			host = site.URL
		}
		if host != "" {
			ips, err := net.LookupIP(host)
			if err != nil {
				return CheckResult{SiteID: site.ID, Status: string(models.StatusDown), ErrorReason: "resolve failed: " + err.Error()}
			}
			for _, ip := range ips {
				if isPrivateIP(ip) {
					return CheckResult{SiteID: site.ID, Status: string(models.StatusDown), ErrorReason: "target resolves to private IP"}
				}
			}
			pinnedIP = ips[0]
		}
	}

	switch site.Type {
	case "http":
		return runHTTPCheck(ctx, site, strict, insecure, globalInsecure)
	case "ping":
		return runPingCheck(ctx, site, pinnedIP)
	case "port":
		return runPortCheck(ctx, site, pinnedIP)
	case "dns":
		return runDNSCheck(ctx, site, allowPrivate)
	default:
		return CheckResult{SiteID: site.ID, Status: string(models.StatusDown), ErrorReason: "unsupported monitor type: " + site.Type}
	}
}

func runHTTPCheck(ctx context.Context, site models.SiteConfig, strict, insecure *http.Client, globalInsecure bool) CheckResult {
	method := site.Method
	if method == "" {
		method = "GET"
	}

	timeout := siteTimeout(site)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, site.URL, nil)
	if err != nil {
		return CheckResult{SiteID: site.ID, Status: string(models.StatusDown), ErrorReason: "invalid request: " + err.Error()}
	}

	client := strict
	if globalInsecure || site.IgnoreTLS {
		client = insecure
	}

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start)

	result := CheckResult{
		SiteID:    site.ID,
		Status:    string(models.StatusUp),
		LatencyNs: latency.Nanoseconds(),
	}

	if err != nil {
		result.Status = string(models.StatusDown)
		result.ErrorReason = truncateError(err.Error(), maxErrorLength)
		return result
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	result.StatusCode = resp.StatusCode
	if !isCodeAccepted(resp.StatusCode, site.AcceptedCodes) {
		result.Status = string(models.StatusDown)
		expected := site.AcceptedCodes
		if expected == "" {
			expected = defaultAcceptedCodes
		}
		result.ErrorReason = fmt.Sprintf("HTTP %d (expected %s)", resp.StatusCode, expected)
	}

	if site.CheckSSL && resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		result.HasSSL = true
		cert := resp.TLS.PeerCertificates[0]
		result.CertExpiry = cert.NotAfter
		if time.Now().After(cert.NotAfter) {
			result.Status = string(models.StatusSSLExp)
			result.ErrorReason = "SSL certificate expired"
		}
	}

	return result
}

func runPingCheck(_ context.Context, site models.SiteConfig, pinnedIP net.IP) CheckResult {
	host := site.Hostname
	if host == "" {
		host = site.URL
	}

	pinger, err := probing.NewPinger(host)
	if err != nil {
		return CheckResult{SiteID: site.ID, Status: string(models.StatusDown), ErrorReason: "ping setup: " + err.Error()}
	}
	if pinnedIP != nil {
		pinger.SetIPAddr(&net.IPAddr{IP: pinnedIP})
	}
	pinger.Count = 1
	pinger.Timeout = siteTimeout(site)
	pinger.SetPrivileged(false)

	start := time.Now()
	err = pinger.Run()
	latency := time.Since(start)

	if err != nil {
		return CheckResult{SiteID: site.ID, Status: string(models.StatusDown), LatencyNs: latency.Nanoseconds(), ErrorReason: "ping failed: " + err.Error()}
	}
	if pinger.Statistics().PacketsRecv == 0 {
		return CheckResult{SiteID: site.ID, Status: string(models.StatusDown), LatencyNs: latency.Nanoseconds(), ErrorReason: "no ICMP response"}
	}

	stats := pinger.Statistics()
	return CheckResult{SiteID: site.ID, Status: string(models.StatusUp), LatencyNs: stats.AvgRtt.Nanoseconds()}
}

func runPortCheck(_ context.Context, site models.SiteConfig, pinnedIP net.IP) CheckResult {
	host := site.Hostname
	if host == "" {
		host = site.URL
	}
	if pinnedIP != nil {
		host = pinnedIP.String()
	}
	addr := net.JoinHostPort(host, strconv.Itoa(site.Port))
	timeout := siteTimeout(site)

	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, timeout)
	latency := time.Since(start)

	if err != nil {
		return CheckResult{SiteID: site.ID, Status: string(models.StatusDown), LatencyNs: latency.Nanoseconds(), ErrorReason: truncateError(err.Error(), maxErrorLength)}
	}
	_ = conn.Close()
	return CheckResult{SiteID: site.ID, Status: string(models.StatusUp), LatencyNs: latency.Nanoseconds()}
}

func runDNSCheck(_ context.Context, site models.SiteConfig, allowPrivate bool) CheckResult {
	host := site.Hostname
	if host == "" {
		host = site.URL
	}

	server := site.DNSServer
	if server == "" {
		server = defaultDNSServer
	}
	serverHost, serverPort, err := net.SplitHostPort(server)
	if err != nil {
		serverHost = server
		serverPort = defaultDNSPort
	}
	if !allowPrivate {
		if serverPort != defaultDNSPort {
			return CheckResult{SiteID: site.ID, Status: string(models.StatusDown), ErrorReason: "DNS server port must be 53"}
		}
		if ips, err := net.LookupIP(serverHost); err == nil {
			for _, ip := range ips {
				if isPrivateIP(ip) {
					return CheckResult{SiteID: site.ID, Status: string(models.StatusDown), ErrorReason: "DNS server resolves to private address"}
				}
			}
		}
	}
	server = net.JoinHostPort(serverHost, serverPort)

	qtype := dns.TypeA
	switch site.DNSResolveType {
	case "AAAA":
		qtype = dns.TypeAAAA
	case "MX":
		qtype = dns.TypeMX
	case "CNAME":
		qtype = dns.TypeCNAME
	case "TXT":
		qtype = dns.TypeTXT
	case "NS":
		qtype = dns.TypeNS
	case "SOA":
		qtype = dns.TypeSOA
	case "SRV":
		qtype = dns.TypeSRV
	case "PTR":
		qtype = dns.TypePTR
	}

	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(host), qtype)

	c := new(dns.Client)
	c.Timeout = siteTimeout(site)

	start := time.Now()
	r, _, err := c.Exchange(m, server)
	latency := time.Since(start)

	if err != nil {
		return CheckResult{SiteID: site.ID, Status: string(models.StatusDown), LatencyNs: latency.Nanoseconds(), ErrorReason: "DNS query failed: " + err.Error()}
	}
	if r.Rcode != dns.RcodeSuccess {
		return CheckResult{SiteID: site.ID, Status: string(models.StatusDown), StatusCode: r.Rcode, LatencyNs: latency.Nanoseconds(), ErrorReason: "DNS RCODE: " + dns.RcodeToString[r.Rcode]}
	}
	return CheckResult{SiteID: site.ID, Status: string(models.StatusUp), LatencyNs: latency.Nanoseconds()}
}

func siteTimeout(site models.SiteConfig) time.Duration {
	if site.Timeout > 0 {
		return time.Duration(site.Timeout) * time.Second
	}
	return defaultTimeout
}

func isCodeAccepted(code int, accepted string) bool {
	if accepted == "" {
		return code >= defaultHTTPStatusMin && code < defaultHTTPStatusMax
	}
	for _, part := range strings.Split(accepted, ",") {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			lo, err1 := strconv.Atoi(strings.TrimSpace(bounds[0]))
			hi, err2 := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err1 == nil && err2 == nil && code >= lo && code <= hi {
				return true
			}
		} else {
			if v, err := strconv.Atoi(part); err == nil && code == v {
				return true
			}
		}
	}
	return false
}

func truncateError(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
