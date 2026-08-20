package web

import (
	"net"
	"net/http"
	"strings"
)

// TrustedProxyAuth authenticates dashboard requests that arrive through a
// trusted reverse proxy (Authentik, Authelia, oauth2-proxy, Traefik
// ForwardAuth, ...). A request is trusted only when its direct TCP peer
// (RemoteAddr, never X-Forwarded-For) is inside one of the configured
// networks AND the configured identity header is non-empty. Requests that
// fail either check fall through to the normal session/Basic Auth flow, so
// spoofed headers from untrusted addresses are ignored.
type TrustedProxyAuth struct {
	cidrs      []*net.IPNet
	userHeader string
}

// NewTrustedProxyAuth builds a TrustedProxyAuth. Returns nil (auth disabled,
// fail closed to local auth) when no networks or no header are configured.
func NewTrustedProxyAuth(cidrs []*net.IPNet, userHeader string) *TrustedProxyAuth {
	userHeader = strings.TrimSpace(userHeader)
	if len(cidrs) == 0 || userHeader == "" {
		return nil
	}
	return &TrustedProxyAuth{cidrs: cidrs, userHeader: userHeader}
}

// TrustedUser returns the proxy-asserted identity when the request comes from
// a trusted proxy and carries the identity header.
func (t *TrustedProxyAuth) TrustedUser(r *http.Request) (string, bool) {
	if t == nil {
		return "", false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return "", false
	}
	trusted := false
	for _, n := range t.cidrs {
		if n.Contains(ip) {
			trusted = true
			break
		}
	}
	if !trusted {
		return "", false
	}
	// Exactly one header value: an append-mode proxy would deliver
	// [client value, proxy value], letting a client smuggle an identity.
	vals := r.Header.Values(t.userHeader)
	if len(vals) != 1 {
		return "", false
	}
	user := strings.TrimSpace(vals[0])
	if user == "" {
		return "", false
	}
	return user, true
}
