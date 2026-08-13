package server_test

import (
	"context"
	"testing"

	"github.com/rengwu/chartr/internal/chartrtest"
)

// docker-deploy ticket 06: CHARTR_PUBLIC_URL extends the host gate and the
// websocket origin allowlist to whichever public URL chartr is reached
// under. Rig-level table-driven coverage so the assertion is on public
// behaviour (a request answered, a handshake accepted) not on internal
// state.
func TestPublicURLEnvOpensHostAndOrigin(t *testing.T) {
	for _, tc := range []struct {
		name     string
		env      string
		host     string
		origin   string
		wantHost int
	}{
		{
			name:     "http-with-remapped-port",
			env:      "http://localhost:9000",
			host:     "localhost:9000",
			origin:   "http://localhost:9000",
			wantHost: 200,
		},
		{
			name:     "https-implicit-port",
			env:      "https://chartr.example.com",
			host:     "chartr.example.com",
			origin:   "https://chartr.example.com",
			wantHost: 200,
		},
		{
			name:     "comma-first",
			env:      "http://a:8787,https://b.example",
			host:     "a:8787",
			origin:   "http://a:8787",
			wantHost: 200,
		},
		{
			name:     "comma-second",
			env:      "http://a:8787,https://b.example",
			host:     "b.example",
			origin:   "https://b.example",
			wantHost: 200,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CHARTR_PUBLIC_URL", tc.env)
			h := chartrtest.Start(t)

			// Host gate: a request bearing the public Host is accepted.
			if code, _ := h.GetWithHost("/api/health", tc.host); code != tc.wantHost {
				t.Errorf("GET /api/health with Host=%q = %d, want %d", tc.host, code, tc.wantHost)
			}

			// Origin allowlist: a websocket handshake bearing the public Origin
			// completes. The rig's DialControlOrigin returns HTTP 101 on
			// success and >= 400 on refusal.
			cc, status := h.DialControlOrigin(context.Background(), tc.origin)
			if cc == nil {
				t.Errorf("dial with Origin=%q refused (status %d), want accepted", tc.origin, status)
			} else {
				cc.Close()
			}
		})
	}
}

// A Host or Origin not in the derived-plus-env allowlist is still rejected —
// no accidental wildcarding.
func TestPublicURLEnvDoesNotWildcard(t *testing.T) {
	t.Setenv("CHARTR_PUBLIC_URL", "http://chartr.example.com:9000")
	h := chartrtest.Start(t)

	if code, _ := h.GetWithHost("/api/health", "attacker.example:9000"); code != 403 {
		t.Errorf("Host=attacker.example:9000 = %d, want 403", code)
	}
	if code, _ := h.GetWithHost("/api/health", "chartr.example.com:9001"); code != 403 {
		t.Errorf("wrong port = %d, want 403", code)
	}
	if cc, _ := h.DialControlOrigin(context.Background(), "http://attacker.example:9000"); cc != nil {
		cc.Close()
		t.Errorf("wrong-origin handshake was accepted")
	}
}

// Malformed-URL fail-fast is covered by the unit tests in publicurl_test.go
// (TestPublicSurfaceMalformedURLIsRejected, TestParsePublicURLList) since a
// rig-level failing-startup assertion is awkward — the rig's cleanup treats
// a Serve error as a test failure, and the failure IS the desired shape.
