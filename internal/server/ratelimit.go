package server

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// maxVisitors caps the rate-limiter map so a flood of distinct keys can't grow
// it without bound. With the trusted-proxy gate below, keys come from real peer
// addresses, so this is a defense-in-depth ceiling rather than the primary
// guard.
const maxVisitors = 10000

type visitor struct {
	tokens   float64
	lastSeen time.Time
}

type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     float64
	burst    float64
	trusted  []*net.IPNet
	stop     chan struct{}
}

func NewRateLimiter(requestsPerMinute int, trusted []*net.IPNet) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     float64(requestsPerMinute) / 60.0,
		burst:    float64(requestsPerMinute),
		trusted:  trusted,
		stop:     make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) Stop() {
	close(rl.stop)
}

func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[ip]
	now := time.Now()

	if !exists {
		if len(rl.visitors) >= maxVisitors {
			rl.evictOldest()
		}
		rl.visitors[ip] = &visitor{tokens: rl.burst - 1, lastSeen: now}
		return true
	}

	elapsed := now.Sub(v.lastSeen).Seconds()
	v.tokens += elapsed * rl.rate
	if v.tokens > rl.burst {
		v.tokens = rl.burst
	}
	v.lastSeen = now

	if v.tokens < 1 {
		return false
	}
	v.tokens--
	return true
}

// evictOldest removes the least-recently-seen visitor. Called only when the map
// is at capacity, so the O(n) scan is rare. Caller holds rl.mu.
func (rl *RateLimiter) evictOldest() {
	var oldestKey string
	var oldest time.Time
	for k, v := range rl.visitors {
		if oldestKey == "" || v.lastSeen.Before(oldest) {
			oldestKey = k
			oldest = v.lastSeen
		}
	}
	if oldestKey != "" {
		delete(rl.visitors, oldestKey)
	}
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			cutoff := time.Now().Add(-10 * time.Minute)
			for ip, v := range rl.visitors {
				if v.lastSeen.Before(cutoff) {
					delete(rl.visitors, ip)
				}
			}
			rl.mu.Unlock()
		case <-rl.stop:
			return
		}
	}
}

// clientIP determines the rate-limit key for a request. X-Forwarded-For is only
// honored when the immediate peer (RemoteAddr) is a configured trusted proxy;
// otherwise the header is attacker-controlled and ignored, so a spoofed XFF
// can't mint unlimited distinct keys (rate-limit bypass + memory DoS). When the
// peer is trusted, the right-most address that is not itself a trusted proxy is
// the real client (RFC 7239 right-most-untrusted-hop).
func clientIP(r *http.Request, trusted []*net.IPNet) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	if len(trusted) == 0 || !ipInCIDRs(net.ParseIP(host), trusted) {
		return host
	}

	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return host
	}
	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.TrimSpace(parts[i]))
		if ip == nil {
			continue
		}
		if !ipInCIDRs(ip, trusted) {
			return ip.String()
		}
	}
	return host
}

func ipInCIDRs(ip net.IP, cidrs []*net.IPNet) bool {
	if ip == nil {
		return false
	}
	for _, c := range cidrs {
		if c.Contains(ip) {
			return true
		}
	}
	return false
}

func RateLimit(limiter *RateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow(clientIP(r, limiter.trusted)) {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}
