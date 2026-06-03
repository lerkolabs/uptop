package tui

import "testing"

func TestClassifyError(t *testing.T) {
	tests := []struct {
		reason     string
		siteType   string
		statusCode int
		want       ErrorCategory
	}{
		{"", "http", 0, ErrCatUnknown},
		{"some unknown error", "http", 0, ErrCatUnknown},

		// PRIV
		{"target resolves to private IP", "http", 0, ErrCatPrivate},
		{"blocked: host resolves to private address 10.0.0.1", "port", 0, ErrCatPrivate},

		// TLS
		{"SSL certificate expired", "http", 0, ErrCatTLS},
		{"tls handshake failure", "http", 0, ErrCatTLS},
		{"x509: certificate has expired", "http", 0, ErrCatTLS},
		{"x509: certificate signed by unknown authority", "http", 0, ErrCatTLS},

		// DNS
		{"DNS query failed: NXDOMAIN", "http", 0, ErrCatDNS},
		{"DNS RCODE: SERVFAIL", "dns", 0, ErrCatDNS},
		{"dial tcp: lookup example.com: no such host", "http", 0, ErrCatDNS},
		{"DNS query failed: server misbehaving", "dns", 0, ErrCatDNS},

		// TMO
		{"dial tcp 1.2.3.4:443: i/o timeout", "http", 0, ErrCatTimeout},
		{"context deadline exceeded", "http", 0, ErrCatTimeout},
		{"Get https://example.com: context canceled", "http", 0, ErrCatTimeout},

		// ICMP
		{"no ICMP response", "ping", 0, ErrCatICMP},
		{"ping setup: permission denied", "ping", 0, ErrCatICMP},
		{"ping failed: packet loss", "ping", 0, ErrCatICMP},

		// TCP
		{"dial tcp 1.2.3.4:80: connect: connection refused", "port", 0, ErrCatTCP},
		{"connection reset by peer", "http", 0, ErrCatTCP},
		{"dial tcp: no route to host", "http", 0, ErrCatTCP},
		{"network unreachable", "http", 0, ErrCatTCP},
		{"failed to connect to 10.0.0.1:443", "http", 0, ErrCatTCP},

		// HTTP
		{"HTTP 500 (expected 200-299)", "http", 500, ErrCatHTTP},
		{"HTTP 403 (expected 200-299)", "http", 403, ErrCatHTTP},
		{"keyword not found", "http", 200, ErrCatHTTP},
		{"", "http", 502, ErrCatUnknown},
		{"unexpected status", "http", 404, ErrCatHTTP},
	}
	for _, tt := range tests {
		got := classifyError(tt.reason, tt.siteType, tt.statusCode)
		if got != tt.want {
			t.Errorf("classifyError(%q, %q, %d) = %q, want %q",
				tt.reason, tt.siteType, tt.statusCode, got, tt.want)
		}
	}
}

func TestCategoryTag(t *testing.T) {
	tests := []struct {
		cat  ErrorCategory
		want string
	}{
		{ErrCatDNS, "[DNS]"},
		{ErrCatTCP, "[TCP]"},
		{ErrCatTLS, "[TLS]"},
		{ErrCatHTTP, "[HTTP]"},
		{ErrCatICMP, "[ICMP]"},
		{ErrCatTimeout, "[TMO]"},
		{ErrCatPrivate, "[PRIV]"},
		{ErrCatUnknown, ""},
	}
	for _, tt := range tests {
		got := categoryTag(tt.cat)
		if got != tt.want {
			t.Errorf("categoryTag(%q) = %q, want %q", tt.cat, got, tt.want)
		}
	}
}

func TestConnectionChain(t *testing.T) {
	t.Run("nil for non-http", func(t *testing.T) {
		if chain := connectionChain("no ICMP response", "ping", 0, false); chain != nil {
			t.Errorf("expected nil for ping, got %v", chain)
		}
	})

	t.Run("nil for empty error", func(t *testing.T) {
		if chain := connectionChain("", "http", 0, true); chain != nil {
			t.Errorf("expected nil for empty error, got %v", chain)
		}
	})

	t.Run("DNS failure HTTPS", func(t *testing.T) {
		chain := connectionChain("no such host", "http", 0, true)
		if len(chain) != 4 {
			t.Fatalf("expected 4 steps, got %d", len(chain))
		}
		if chain[0].Status != stepFailed {
			t.Errorf("DNS step: want failed, got %d", chain[0].Status)
		}
		if chain[1].Status != stepSkipped {
			t.Errorf("TCP step: want skipped, got %d", chain[1].Status)
		}
		if chain[2].Status != stepSkipped {
			t.Errorf("TLS step: want skipped, got %d", chain[2].Status)
		}
		if chain[3].Status != stepSkipped {
			t.Errorf("HTTP step: want skipped, got %d", chain[3].Status)
		}
	})

	t.Run("TCP failure HTTP", func(t *testing.T) {
		chain := connectionChain("connection refused", "http", 0, false)
		if len(chain) != 3 {
			t.Fatalf("expected 3 steps (no TLS), got %d", len(chain))
		}
		if chain[0].Status != stepPassed {
			t.Errorf("DNS step: want passed, got %d", chain[0].Status)
		}
		if chain[1].Status != stepFailed {
			t.Errorf("TCP step: want failed, got %d", chain[1].Status)
		}
		if chain[2].Status != stepSkipped {
			t.Errorf("HTTP step: want skipped, got %d", chain[2].Status)
		}
	})

	t.Run("TLS failure HTTPS", func(t *testing.T) {
		chain := connectionChain("x509: certificate has expired", "http", 0, true)
		if len(chain) != 4 {
			t.Fatalf("expected 4 steps, got %d", len(chain))
		}
		if chain[0].Status != stepPassed {
			t.Errorf("DNS: want passed, got %d", chain[0].Status)
		}
		if chain[1].Status != stepPassed {
			t.Errorf("TCP: want passed, got %d", chain[1].Status)
		}
		if chain[2].Status != stepFailed {
			t.Errorf("TLS: want failed, got %d", chain[2].Status)
		}
		if chain[3].Status != stepSkipped {
			t.Errorf("HTTP: want skipped, got %d", chain[3].Status)
		}
	})

	t.Run("HTTP failure HTTPS", func(t *testing.T) {
		chain := connectionChain("HTTP 500 (expected 200-299)", "http", 500, true)
		if len(chain) != 4 {
			t.Fatalf("expected 4 steps, got %d", len(chain))
		}
		for i := 0; i < 3; i++ {
			if chain[i].Status != stepPassed {
				t.Errorf("step %d: want passed, got %d", i, chain[i].Status)
			}
		}
		if chain[3].Status != stepFailed {
			t.Errorf("HTTP: want failed, got %d", chain[3].Status)
		}
		if chain[3].Detail != "HTTP 500" {
			t.Errorf("HTTP detail: want %q, got %q", "HTTP 500", chain[3].Detail)
		}
	})

	t.Run("timeout maps to TCP step", func(t *testing.T) {
		chain := connectionChain("i/o timeout", "http", 0, true)
		if chain[1].Status != stepFailed {
			t.Errorf("TCP step: want failed for timeout, got %d", chain[1].Status)
		}
		if chain[1].Detail != "i/o timeout" {
			t.Errorf("detail: want %q, got %q", "i/o timeout", chain[1].Detail)
		}
	})

	t.Run("private IP maps to DNS step", func(t *testing.T) {
		chain := connectionChain("target resolves to private IP", "http", 0, true)
		if chain[0].Status != stepFailed {
			t.Errorf("DNS step: want failed for private IP, got %d", chain[0].Status)
		}
	})
}
