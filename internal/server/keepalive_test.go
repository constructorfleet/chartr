package server_test

import (
	"context"
	"testing"
	"time"

	"github.com/rengwu/chartr/internal/chartrtest"
	"github.com/rengwu/chartr/internal/server"
)

// docker-deploy ticket 03: the server sends keepalive pings on control and
// terminal websockets so an idle reverse-proxy doesn't drop long-lived
// connections. The interval is package-scoped so the test can shorten it
// from 30s to something the suite can wait on.
func TestControlSocketSendsKeepalivePings(t *testing.T) {
	t.Cleanup(server.SetPingIntervalForTest(50 * time.Millisecond))
	before := server.PingsSentForTest()

	h := chartrtest.Start(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn := h.DialControl(ctx)
	defer conn.Close()

	// Hold the socket open long enough that at least two pings should have
	// fired at a 50ms interval. Poll the counter rather than sleep-once so a
	// slow CI machine still sees the pings.
	deadline := time.Now().Add(2 * time.Second)
	var got int64
	for time.Now().Before(deadline) {
		got = server.PingsSentForTest() - before
		if got >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got < 2 {
		t.Fatalf("saw %d pings, want ≥ 2 within 2s at 50ms interval", got)
	}
}

func TestTerminalSocketSendsKeepalivePings(t *testing.T) {
	t.Cleanup(server.SetPingIntervalForTest(50 * time.Millisecond))
	h := chartrtest.Start(t)

	// Register a space so we can open a terminal in it — the smallest path
	// to a live /ws/terminal handshake.
	dir := t.TempDir()
	if code, body := h.Post("/api/spaces", map[string]string{"path": dir}); code != 200 {
		t.Fatalf("register space: %d %s", code, body)
	}
	m := h.Snapshot(context.Background())
	if len(m.Spaces) == 0 {
		t.Fatalf("expected a space to appear")
	}
	spaceID := m.Spaces[0].ID
	termID := h.OpenTerminal(spaceID)

	before := server.PingsSentForTest()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tc := h.DialTerminal(ctx, termID)
	defer tc.Close()

	deadline := time.Now().Add(2 * time.Second)
	var got int64
	for time.Now().Before(deadline) {
		got = server.PingsSentForTest() - before
		if got >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got < 2 {
		t.Fatalf("saw %d terminal pings, want ≥ 2 within 2s at 50ms interval", got)
	}
}
