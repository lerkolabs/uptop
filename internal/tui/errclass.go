package tui

import (
	"fmt"
	"strings"
)

type ErrorCategory string

const (
	ErrCatDNS     ErrorCategory = "DNS"
	ErrCatTCP     ErrorCategory = "TCP"
	ErrCatTLS     ErrorCategory = "TLS"
	ErrCatHTTP    ErrorCategory = "HTTP"
	ErrCatICMP    ErrorCategory = "ICMP"
	ErrCatTimeout ErrorCategory = "TMO"
	ErrCatPrivate ErrorCategory = "PRIV"
	ErrCatUnknown ErrorCategory = ""
)

func classifyError(errorReason string, siteType string, statusCode int) ErrorCategory {
	if errorReason == "" {
		return ErrCatUnknown
	}
	lower := strings.ToLower(errorReason)

	if strings.Contains(lower, "resolves to private") || strings.Contains(lower, "private address") || strings.Contains(lower, "private ip") {
		return ErrCatPrivate
	}

	if strings.Contains(lower, "tls handshake") || strings.Contains(lower, "x509") ||
		strings.Contains(lower, "certificate") || strings.Contains(lower, "ssl") {
		return ErrCatTLS
	}

	if strings.Contains(lower, "no such host") || strings.Contains(lower, "nxdomain") ||
		strings.Contains(lower, "dns query") || strings.Contains(lower, "dns rcode") ||
		strings.Contains(lower, "servfail") || strings.Contains(lower, "server misbehaving") {
		return ErrCatDNS
	}

	if strings.Contains(lower, "i/o timeout") || strings.Contains(lower, "deadline exceeded") ||
		strings.Contains(lower, "context canceled") {
		return ErrCatTimeout
	}

	if strings.Contains(lower, "no icmp response") || strings.Contains(lower, "ping setup") ||
		strings.Contains(lower, "ping failed") {
		return ErrCatICMP
	}

	if strings.Contains(lower, "connection refused") || strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "no route to host") || strings.Contains(lower, "network unreachable") ||
		strings.Contains(lower, "network is unreachable") {
		return ErrCatTCP
	}

	if strings.HasPrefix(lower, "http ") || strings.Contains(lower, "keyword not found") {
		return ErrCatHTTP
	}
	if statusCode >= 400 {
		return ErrCatHTTP
	}

	return ErrCatUnknown
}

func categoryTag(cat ErrorCategory) string {
	if cat == ErrCatUnknown {
		return ""
	}
	return "[" + string(cat) + "]"
}

type stepStatus int

const (
	stepPassed stepStatus = iota
	stepFailed
	stepSkipped
)

type chainStep struct {
	Name   string
	Status stepStatus
	Detail string
}

func connectionChain(errorReason string, siteType string, statusCode int, isHTTPS bool) []chainStep {
	if siteType != "http" {
		return nil
	}
	if errorReason == "" {
		return nil
	}

	cat := classifyError(errorReason, siteType, statusCode)
	detail := extractDetail(errorReason, cat, statusCode)

	steps := []struct {
		name string
		cat  ErrorCategory
	}{
		{"DNS resolve", ErrCatDNS},
		{"TCP connect", ErrCatTCP},
	}
	if isHTTPS {
		steps = append(steps, struct {
			name string
			cat  ErrorCategory
		}{"TLS handshake", ErrCatTLS})
	}
	steps = append(steps, struct {
		name string
		cat  ErrorCategory
	}{"HTTP response", ErrCatHTTP})

	chain := make([]chainStep, 0, len(steps))
	failed := false
	for _, s := range steps {
		switch {
		case failed:
			chain = append(chain, chainStep{Name: s.name, Status: stepSkipped, Detail: "(skipped)"})
		case s.cat == cat || (cat == ErrCatTimeout && s.cat == ErrCatTCP) || (cat == ErrCatPrivate && s.cat == ErrCatDNS):
			chain = append(chain, chainStep{Name: s.name, Status: stepFailed, Detail: detail})
			failed = true
		default:
			chain = append(chain, chainStep{Name: s.name, Status: stepPassed})
		}
	}

	return chain
}

func extractDetail(errorReason string, cat ErrorCategory, statusCode int) string {
	lower := strings.ToLower(errorReason)
	var detail string

	switch cat {
	case ErrCatDNS:
		for _, keyword := range []string{"NXDOMAIN", "SERVFAIL", "REFUSED", "no such host"} {
			if strings.Contains(strings.ToUpper(errorReason), strings.ToUpper(keyword)) {
				detail = keyword
				break
			}
		}
		if detail == "" {
			detail = limitStr(errorReason, 30)
		}
	case ErrCatTCP:
		for _, keyword := range []string{"connection refused", "connection reset", "no route to host", "network unreachable"} {
			if strings.Contains(lower, keyword) {
				detail = keyword
				break
			}
		}
		if detail == "" {
			detail = limitStr(errorReason, 30)
		}
	case ErrCatTLS:
		for _, keyword := range []string{"certificate expired", "certificate has expired", "handshake failure", "unknown authority"} {
			if strings.Contains(lower, keyword) {
				detail = keyword
				break
			}
		}
		if detail == "" && strings.Contains(lower, "x509:") {
			idx := strings.Index(lower, "x509:")
			detail = limitStr(errorReason[idx:], 30)
		}
		if detail == "" {
			detail = limitStr(errorReason, 30)
		}
	case ErrCatHTTP:
		if statusCode > 0 {
			detail = fmt.Sprintf("HTTP %d", statusCode)
		} else {
			detail = limitStr(errorReason, 30)
		}
	case ErrCatTimeout:
		if strings.Contains(lower, "i/o timeout") {
			detail = "i/o timeout"
		} else {
			detail = "deadline exceeded"
		}
	case ErrCatICMP:
		if strings.Contains(lower, "no icmp response") {
			detail = "no response"
		} else {
			detail = limitStr(errorReason, 30)
		}
	case ErrCatPrivate:
		detail = "private IP blocked"
	default:
		detail = limitStr(errorReason, 30)
	}

	return detail
}
