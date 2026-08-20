package web

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/onllm-dev/onwatch/v2/internal/config"
)

func testTrustedProxyAuth(t *testing.T, cidrs []string, header string) *TrustedProxyAuth {
	t.Helper()
	nets, err := config.ParseTrustedProxyCIDRs(cidrs)
	if err != nil {
		t.Fatalf("ParseTrustedProxyCIDRs() failed: %v", err)
	}
	return NewTrustedProxyAuth(nets, header)
}

func TestNewTrustedProxyAuth_FailClosed(t *testing.T) {
	if got := NewTrustedProxyAuth(nil, "X-Forwarded-User"); got != nil {
		t.Error("expected nil TrustedProxyAuth without CIDRs")
	}
	nets, _ := config.ParseTrustedProxyCIDRs([]string{"127.0.0.1"})
	if got := NewTrustedProxyAuth(nets, ""); got != nil {
		t.Error("expected nil TrustedProxyAuth without a user header")
	}
}

func TestTrustedProxyAuth_TrustedUser(t *testing.T) {
	tp := testTrustedProxyAuth(t, []string{"172.30.0.0/16", "::1"}, "X-Forwarded-User")

	cases := []struct {
		name       string
		remoteAddr string
		headerVal  string
		wantOK     bool
	}{
		{"trusted IP with header", "172.30.0.5:44321", "alice", true},
		{"trusted IPv6 with header", "[::1]:9999", "alice", true},
		{"trusted IP without header", "172.30.0.5:44321", "", false},
		{"trusted IP with whitespace header", "172.30.0.5:44321", "   ", false},
		{"untrusted IP with spoofed header", "10.0.0.9:5000", "alice", false},
		{"garbage remote addr", "not-an-ip", "alice", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tc.remoteAddr
			if tc.headerVal != "" {
				r.Header.Set("X-Forwarded-User", tc.headerVal)
			}
			user, ok := tp.TrustedUser(r)
			if ok != tc.wantOK {
				t.Errorf("TrustedUser() ok = %v, want %v", ok, tc.wantOK)
			}
			if tc.wantOK && user != "alice" {
				t.Errorf("TrustedUser() user = %q, want alice", user)
			}
		})
	}
}

func TestTrustedProxyAuth_MultiValueHeaderRejected(t *testing.T) {
	tp := testTrustedProxyAuth(t, []string{"172.30.0.0/16"}, "X-Forwarded-User")

	// An append-mode proxy delivers [client value, proxy value]; taking the
	// first would use the client's smuggled identity, so reject outright.
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "172.30.0.5:44321"
	r.Header.Add("X-Forwarded-User", "attacker")
	r.Header.Add("X-Forwarded-User", "alice")
	if _, ok := tp.TrustedUser(r); ok {
		t.Error("multi-value identity header must be rejected")
	}
}

func TestTrustedProxyAuth_NilReceiver(t *testing.T) {
	var tp *TrustedProxyAuth
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Forwarded-User", "alice")
	if _, ok := tp.TrustedUser(r); ok {
		t.Error("nil TrustedProxyAuth must never authenticate")
	}
}

// trustedProxyTestServer builds the session-auth middleware with an optional
// trusted proxy config over a handler that returns 200 "ok".
func trustedProxyTestServer(t *testing.T, tp *TrustedProxyAuth) http.Handler {
	t.Helper()
	sessions := NewSessionStore("admin", legacyHashPassword("secret"), nil)
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	return sessionAuthMiddlewareWithTrustedProxy(sessions, "", tp)(okHandler)
}

func TestSessionAuth_TrustedProxyBypassesLogin(t *testing.T) {
	h := trustedProxyTestServer(t, testTrustedProxyAuth(t, []string{"172.30.0.0/16"}, "X-Forwarded-User"))

	for _, path := range []string{"/", "/api/current"} {
		r := httptest.NewRequest("GET", path, nil)
		r.RemoteAddr = "172.30.0.5:12345"
		r.Header.Set("X-Forwarded-User", "alice")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", path, w.Code)
		}
	}
}

func TestSessionAuth_SpoofedHeaderFromUntrustedIPRejected(t *testing.T) {
	h := trustedProxyTestServer(t, testTrustedProxyAuth(t, []string{"172.30.0.0/16"}, "X-Forwarded-User"))

	// Browser route: redirected to login despite spoofed header
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.9:12345"
	r.Header.Set("X-Forwarded-User", "alice")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusFound {
		t.Errorf("dashboard: status = %d, want 302 redirect to login", w.Code)
	}

	// API route: 401 despite spoofed header
	r = httptest.NewRequest("GET", "/api/current", nil)
	r.RemoteAddr = "10.0.0.9:12345"
	r.Header.Set("X-Forwarded-User", "alice")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("api: status = %d, want 401", w.Code)
	}
}

func TestSessionAuth_TrustedIPWithoutHeaderFallsBack(t *testing.T) {
	h := trustedProxyTestServer(t, testTrustedProxyAuth(t, []string{"172.30.0.0/16"}, "X-Forwarded-User"))

	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "172.30.0.5:12345"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302 redirect to login", w.Code)
	}
}

func TestSessionAuth_APIBasicAuthStillWorksInTrustedMode(t *testing.T) {
	h := trustedProxyTestServer(t, testTrustedProxyAuth(t, []string{"172.30.0.0/16"}, "X-Forwarded-User"))

	// Direct API caller (untrusted IP) with valid Basic Auth still succeeds.
	r := httptest.NewRequest("GET", "/api/current", nil)
	r.RemoteAddr = "10.0.0.9:12345"
	r.SetBasicAuth("admin", "secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for valid Basic Auth", w.Code)
	}
}

func TestSessionAuth_LocalModeUnaffected(t *testing.T) {
	h := trustedProxyTestServer(t, nil)

	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "172.30.0.5:12345"
	r.Header.Set("X-Forwarded-User", "alice")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302 (trusted proxy disabled)", w.Code)
	}
}

func TestParseTrustedProxyCIDRs_UsedByWeb(t *testing.T) {
	nets, err := config.ParseTrustedProxyCIDRs([]string{"127.0.0.1/32"})
	if err != nil {
		t.Fatalf("ParseTrustedProxyCIDRs() failed: %v", err)
	}
	if !nets[0].Contains(net.ParseIP("127.0.0.1")) {
		t.Error("expected 127.0.0.1 to be contained")
	}
}
