package monitor

import (
	"context"
	"fmt"
	"net"
	"time"
)

var privateRanges []*net.IPNet

func init() {
	cidrs := []string{
		"0.0.0.0/8",
		"127.0.0.0/8",
		"::1/128",
		"10.0.0.0/8",
		"100.64.0.0/10",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"fe80::/10",
		"fc00::/7",
	}
	for _, cidr := range cidrs {
		_, network, _ := net.ParseCIDR(cidr)
		privateRanges = append(privateRanges, network)
	}
}

func isPrivateIP(ip net.IP) bool {
	if ip.IsUnspecified() || ip.IsMulticast() || ip.IsLoopback() {
		return true
	}
	for _, network := range privateRanges {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func SafeDialContext(allowPrivate bool) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}

		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}

		if !allowPrivate {
			for _, ip := range ips {
				if isPrivateIP(ip.IP) {
					return nil, fmt.Errorf("blocked: %s resolves to private address %s", host, ip.IP)
				}
			}
		}

		dialer := &net.Dialer{Timeout: 10 * time.Second}
		for _, ip := range ips {
			target := net.JoinHostPort(ip.IP.String(), port)
			conn, err := dialer.DialContext(ctx, network, target)
			if err == nil {
				return conn, nil
			}
		}
		return nil, fmt.Errorf("failed to connect to %s", addr)
	}
}
