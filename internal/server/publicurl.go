package server

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// publicSurface composes chartr's every-route host allowlist and the
// websocket origin patterns from two ingredients: the address the listener
// actually bound, and zero or more "public URLs" — the additional
// (scheme, host, port) triples requests may arrive under when chartr is
// reached through a reverse proxy or a Docker-remapped host port.
//
// The base rule and pattern list stay byte-identical to the address-derived
// pair when extras is empty (docker-deploy, ticket 05); ticket 06 wires
// CHARTR_PUBLIC_URL into extras without touching this seam again.
type publicSurface struct {
	base    hostRule
	extras  []extraAuthority
	Origins []string
}

// extraAuthority is one publicSurface allowance: a normalized name or an IP
// literal, plus the port a request must carry. The scheme lives only on the
// origins list — the host gate does not police scheme.
type extraAuthority struct {
	// host is a normalized lowercase name with no trailing dot and no
	// brackets; empty when this entry is an IP literal (ip is then set).
	host string
	ip   net.IP
	// port is what the request's Host header must carry — the URL's explicit
	// port or, if none, the scheme's default (80 for http, 443 for https).
	port string
	// implicitPort is true when the original URL had no explicit port.
	// A request Host of just "chartr.example.com" (no port) then admits, as
	// well as one carrying the scheme's default explicitly — matching the
	// two spellings a browser produces for the same URL.
	implicitPort bool
}

// newPublicSurface builds the gate's Host allowlist and websocket
// OriginPatterns from the bound address and the optional extras.
//
// The zero-extras call is exactly the composition Serve used to build in-line
// (hostRuleFor + originPatterns), so a call with a nil or empty extras leaves
// behaviour byte-identical to the pre-adapter code.
func newPublicSurface(addr net.Addr, extras []*url.URL) (publicSurface, error) {
	base := hostRuleFor(addr)
	origins := originPatterns(addr)

	authorities := make([]extraAuthority, 0, len(extras))
	for _, u := range extras {
		if u == nil {
			continue
		}
		a, origin, err := parsePublicURL(u)
		if err != nil {
			return publicSurface{}, err
		}
		authorities = append(authorities, a)
		origins = append(origins, origin)
	}

	return publicSurface{base: base, extras: authorities, Origins: origins}, nil
}

// Allows reports whether the request's Host header is admitted by either the
// base rule or one of the extras.
func (p publicSurface) Allows(hostHeader string) bool {
	if p.base.allows(hostHeader) {
		return true
	}
	if len(p.extras) == 0 {
		return false
	}
	host, port, err := net.SplitHostPort(hostHeader)
	portGiven := err == nil
	if !portGiven {
		host = hostHeader
	}
	host = normalizeHost(host)
	ip := net.ParseIP(host)
	for _, a := range p.extras {
		if portGiven {
			if a.port != port {
				continue
			}
		} else if !a.implicitPort {
			// The extra had an explicit port; a portless Host does not match.
			continue
		}
		if a.ip != nil {
			if ip != nil && a.ip.Equal(ip) {
				return true
			}
			continue
		}
		if host == a.host {
			return true
		}
	}
	return false
}

// parsePublicURL takes one user-supplied public URL and returns the gate
// authority to admit and the origin pattern to add to the websocket
// allowlist. A malformed URL (no scheme, no host, wildcard glob) is an
// error, not a silent skip — misconfiguration should fail loudly at
// startup rather than open a hole quietly (docker-deploy, ticket 06 wires
// this in; ticket 05 exposes the seam).
func parsePublicURL(u *url.URL) (extraAuthority, string, error) {
	if u.Scheme != "http" && u.Scheme != "https" {
		return extraAuthority{}, "", fmt.Errorf("public URL %q: scheme must be http or https", u.String())
	}
	if strings.ContainsAny(u.Host, "*?") {
		return extraAuthority{}, "", fmt.Errorf("public URL %q: wildcards are not allowed in the host", u.String())
	}
	host := u.Hostname()
	if host == "" {
		return extraAuthority{}, "", fmt.Errorf("public URL %q: host is required", u.String())
	}
	port := u.Port()
	implicit := port == ""
	if implicit {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	a := extraAuthority{port: port, implicitPort: implicit}
	host = normalizeHost(host)
	if ip := net.ParseIP(host); ip != nil {
		a.ip = ip
	} else {
		a.host = host
	}
	// The origin pattern is the exact scheme://host[:port] shape a browser's
	// Origin header carries. Default-port URLs must not carry the port in
	// the origin (a browser omits it), so build it with the raw Host when
	// it had one and without when it did not.
	var origin string
	if u.Port() != "" {
		origin = u.Scheme + "://" + normalizeHost(u.Hostname()) + ":" + u.Port()
	} else {
		origin = u.Scheme + "://" + normalizeHost(u.Hostname())
	}
	return a, origin, nil
}

// parsePublicURLList splits raw on commas and parses each entry. Empty
// entries (a leading/trailing comma, or a completely empty raw) yield no
// URLs. Callers pass the result to newPublicSurface as the extras list.
func parsePublicURLList(raw string) ([]*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]*url.URL, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		u, err := url.Parse(p)
		if err != nil {
			return nil, fmt.Errorf("public URL %q: %w", p, err)
		}
		out = append(out, u)
	}
	return out, nil
}
