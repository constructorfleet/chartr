package server

import (
	"net"
	"net/url"
	"testing"
)

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return u
}

// Composition with an empty extras list is exactly the pair of derived
// values Serve used to build in-line — this is the invariant the ticket-05
// prefactor rides on (docker-deploy, ticket 05).
func TestPublicSurfaceEmptyExtrasMatchesDerived(t *testing.T) {
	addr := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8787}

	surface, err := newPublicSurface(addr, nil)
	if err != nil {
		t.Fatalf("newPublicSurface: %v", err)
	}

	wantOrigins := originPatterns(addr)
	if len(surface.Origins) != len(wantOrigins) {
		t.Fatalf("origins: got %v, want %v", surface.Origins, wantOrigins)
	}
	for i, o := range wantOrigins {
		if surface.Origins[i] != o {
			t.Fatalf("origin[%d]: got %q, want %q", i, surface.Origins[i], o)
		}
	}

	rule := hostRuleFor(addr)
	for _, hh := range []string{"127.0.0.1:8787", "localhost:8787", "[::1]:8787"} {
		gotBase := rule.allows(hh)
		gotSurface := surface.Allows(hh)
		if gotBase != gotSurface {
			t.Errorf("Host %q: base rule %v, surface %v — should match", hh, gotBase, gotSurface)
		}
	}
	if surface.Allows("evil.example:8787") {
		t.Errorf("evil.example admitted by empty-extras surface")
	}
}

// One extra URL adds its host+port to the gate allowlist and its
// scheme+host+port to the origin patterns — the shape ticket 06 will use.
func TestPublicSurfaceExtraURLIsAdmitted(t *testing.T) {
	addr := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8787}
	u := mustURL(t, "http://chartr.example.com:9000")

	surface, err := newPublicSurface(addr, []*url.URL{u})
	if err != nil {
		t.Fatalf("newPublicSurface: %v", err)
	}

	if !surface.Allows("chartr.example.com:9000") {
		t.Errorf("extra URL not admitted by Allows")
	}
	if surface.Allows("chartr.example.com:9001") {
		t.Errorf("wrong port admitted")
	}
	if surface.Allows("other.example.com:9000") {
		t.Errorf("wrong host admitted")
	}

	// The base loopback rule still stands.
	if !surface.Allows("127.0.0.1:8787") {
		t.Errorf("base rule 127.0.0.1:8787 was lost")
	}

	found := false
	for _, o := range surface.Origins {
		if o == "http://chartr.example.com:9000" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("extra origin pattern missing from %v", surface.Origins)
	}
}

// An https URL with the default 443 admits a bare (portless) Host header —
// what a browser sends when the URL had no explicit port.
func TestPublicSurfaceHTTPSDefaultPort(t *testing.T) {
	addr := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8787}
	u := mustURL(t, "https://chartr.example.com")

	surface, err := newPublicSurface(addr, []*url.URL{u})
	if err != nil {
		t.Fatalf("newPublicSurface: %v", err)
	}
	if !surface.Allows("chartr.example.com") {
		t.Errorf("bare host was not admitted for https URL")
	}
	if !surface.Allows("chartr.example.com:443") {
		t.Errorf("explicit :443 was not admitted for https URL")
	}
	// The origin pattern must NOT carry :443 — browsers omit the default port.
	for _, o := range surface.Origins {
		if o == "https://chartr.example.com:443" {
			t.Errorf("https origin pattern carried a redundant :443: %v", surface.Origins)
		}
	}
}

// Malformed public URLs are rejected — a fail-fast at startup, not a silent
// hole (docker-deploy, ticket 06 makes this reachable from a user; ticket 05
// pins the shape).
func TestPublicSurfaceMalformedURLIsRejected(t *testing.T) {
	addr := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8787}
	for _, tc := range []struct {
		name string
		raw  string
	}{
		{name: "no scheme", raw: "chartr.example.com:9000"},
		{name: "no host", raw: "http://"},
		{name: "wildcard", raw: "https://*.example.com"},
		{name: "wrong scheme", raw: "ftp://chartr.example.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := mustURL(t, tc.raw)
			if _, err := newPublicSurface(addr, []*url.URL{u}); err == nil {
				t.Fatalf("%q was accepted; want an error", tc.raw)
			}
		})
	}
}

// The comma-separated parser produces one URL per entry and skips whitespace
// and blanks.
func TestParsePublicURLList(t *testing.T) {
	list, err := parsePublicURLList(" http://a:8787 , https://b.example ,, http://c ")
	if err != nil {
		t.Fatalf("parsePublicURLList: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("got %d URLs, want 3: %v", len(list), list)
	}
	wantHosts := []string{"a", "b.example", "c"}
	for i, u := range list {
		if u.Hostname() != wantHosts[i] {
			t.Errorf("URL[%d] host = %q, want %q", i, u.Hostname(), wantHosts[i])
		}
	}
}

func TestParsePublicURLListEmpty(t *testing.T) {
	if list, err := parsePublicURLList(""); err != nil || len(list) != 0 {
		t.Fatalf("empty list: got %v, %v", list, err)
	}
}
