package monitor

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

func TestRunCheck_HTTP_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	site := models.SiteConfig{ID: 1, Type: "http", URL: srv.URL}
	result := RunCheck(context.Background(), site, http.DefaultClient, http.DefaultClient, false, false)

	if result.Status != "UP" {
		t.Errorf("expected UP, got %s", result.Status)
	}
	if result.StatusCode != 200 {
		t.Errorf("expected 200, got %d", result.StatusCode)
	}
	if result.LatencyNs <= 0 {
		t.Error("expected positive latency")
	}
}

func TestRunCheck_HTTP_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	site := models.SiteConfig{ID: 1, Type: "http", URL: srv.URL}
	result := RunCheck(context.Background(), site, http.DefaultClient, http.DefaultClient, false, false)

	if result.Status != "DOWN" {
		t.Errorf("expected DOWN, got %s", result.Status)
	}
	if result.StatusCode != 500 {
		t.Errorf("expected 500, got %d", result.StatusCode)
	}
}

func TestRunCheck_HTTP_CustomAcceptedCodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(302)
	}))
	defer srv.Close()

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	site := models.SiteConfig{ID: 1, Type: "http", URL: srv.URL, AcceptedCodes: "200-399"}
	result := RunCheck(context.Background(), site, client, client, false, false)

	if result.Status != "UP" {
		t.Errorf("expected UP with accepted 200-399, got %s", result.Status)
	}
}

func TestRunCheck_HTTP_MethodRespected(t *testing.T) {
	var receivedMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(200)
	}))
	defer srv.Close()

	site := models.SiteConfig{ID: 1, Type: "http", URL: srv.URL, Method: "HEAD"}
	RunCheck(context.Background(), site, http.DefaultClient, http.DefaultClient, false, false)

	if receivedMethod != "HEAD" {
		t.Errorf("expected HEAD, got %s", receivedMethod)
	}
}

func TestRunCheck_HTTP_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	site := models.SiteConfig{ID: 1, Type: "http", URL: srv.URL, Timeout: 1}
	result := RunCheck(context.Background(), site, http.DefaultClient, http.DefaultClient, false, false)

	if result.Status != "DOWN" {
		t.Errorf("expected DOWN on timeout, got %s", result.Status)
	}
}

func TestRunCheck_HTTP_SSLFields(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	insecureClient := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}

	site := models.SiteConfig{ID: 1, Type: "http", URL: srv.URL, CheckSSL: true, IgnoreTLS: true}
	result := RunCheck(context.Background(), site, http.DefaultClient, insecureClient, false, false)

	if result.Status != "UP" {
		t.Errorf("expected UP, got %s", result.Status)
	}
	if !result.HasSSL {
		t.Error("expected HasSSL=true")
	}
	if result.CertExpiry.IsZero() {
		t.Error("expected CertExpiry populated")
	}
}

func TestRunCheck_Port_Open(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)

	site := models.SiteConfig{ID: 1, Type: "port", Hostname: "127.0.0.1", Port: port, Timeout: 2}
	result := RunCheck(context.Background(), site, nil, nil, false, true)

	if result.Status != "UP" {
		t.Errorf("expected UP, got %s", result.Status)
	}
	if result.LatencyNs <= 0 {
		t.Error("expected positive latency")
	}
}

func TestRunCheck_Port_Closed(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	ln.Close()

	site := models.SiteConfig{ID: 1, Type: "port", Hostname: "127.0.0.1", Port: port, Timeout: 1}
	result := RunCheck(context.Background(), site, nil, nil, false, true)

	if result.Status != "DOWN" {
		t.Errorf("expected DOWN, got %s", result.Status)
	}
}

func TestRunCheck_Port_BlocksPrivateByDefault(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)

	site := models.SiteConfig{ID: 1, Type: "port", Hostname: "127.0.0.1", Port: port, Timeout: 2}
	result := RunCheck(context.Background(), site, nil, nil, false, false)

	if result.Status != "DOWN" {
		t.Errorf("expected DOWN when private targets blocked, got %s", result.Status)
	}
}

func TestRunCheck_UnknownType(t *testing.T) {
	site := models.SiteConfig{ID: 1, Type: "invalid"}
	result := RunCheck(context.Background(), site, nil, nil, false, false)

	if result.Status != "DOWN" {
		t.Errorf("expected DOWN for unknown type, got %s", result.Status)
	}
}

func TestIsCodeAccepted(t *testing.T) {
	tests := []struct {
		code     int
		accepted string
		want     bool
	}{
		{200, "", true},
		{299, "", true},
		{300, "", false},
		{302, "200-399", true},
		{400, "200-399", false},
		{301, "200,301,404", true},
		{500, "200,301,404", false},
		{404, "200-299,400-499", true},
		{500, "200-299,400-499", false},
	}

	for _, tt := range tests {
		got := isCodeAccepted(tt.code, tt.accepted)
		if got != tt.want {
			t.Errorf("isCodeAccepted(%d, %q) = %v, want %v", tt.code, tt.accepted, got, tt.want)
		}
	}
}

func TestSiteTimeout(t *testing.T) {
	if got := siteTimeout(models.SiteConfig{Timeout: 0}); got != 5*time.Second {
		t.Errorf("expected 5s default, got %v", got)
	}
	if got := siteTimeout(models.SiteConfig{Timeout: 10}); got != 10*time.Second {
		t.Errorf("expected 10s, got %v", got)
	}
}
