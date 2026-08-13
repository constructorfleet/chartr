package server_test

import (
	"bufio"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rengwu/chartr/internal/chartrtest"
	"github.com/rengwu/chartr/internal/server"
)

// docker-deploy ticket 04: a slow-drip request-header stream is dropped by
// the server rather than holding a connection open indefinitely. The
// timeout is package-scoped so this test can shorten it; the production
// value stays 10s.
func TestReadHeaderTimeoutDropsASlowDripHeader(t *testing.T) {
	t.Cleanup(server.SetReadHeaderTimeoutForTest(150 * time.Millisecond))

	h := chartrtest.Start(t)

	// Answered request first, to prove the server got past the point that
	// installed the ReadHeaderTimeout on its http.Server.
	if code, _ := h.Get("/api/health"); code != 200 {
		t.Fatalf("sanity: /api/health = %d, want 200", code)
	}

	authority := strings.TrimPrefix(h.BaseURL, "http://")
	conn, err := net.DialTimeout("tcp", authority, 2*time.Second)
	if err != nil {
		t.Fatalf("dial %s: %v", authority, err)
	}
	defer conn.Close()

	// Write the request line and then trickle header bytes one at a time,
	// slowly enough that the whole header stream takes longer than the
	// configured timeout. ReadHeaderTimeout caps the total time to the blank
	// line that ends the headers.
	if _, err := conn.Write([]byte("GET /api/health HTTP/1.1\r\nHost: " + authority + "\r\n")); err != nil {
		t.Fatalf("write request line: %v", err)
	}
	drip := []byte("X-Drip: 0123456789\r\n\r\n")
	go func() {
		for _, b := range drip {
			if _, err := conn.Write([]byte{b}); err != nil {
				return
			}
			time.Sleep(30 * time.Millisecond)
		}
	}()

	// The server must have refused the request: either the connection was
	// closed before any response (EOF from ReadResponse) or an error
	// response (408 Request Timeout) came back. A 200 means the slow-drip
	// was tolerated and the timeout did not take.
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodGet})
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Fatalf("slow-drip header was answered 200; timeout did not fire")
		}
		if resp.StatusCode < 400 {
			t.Fatalf("slow-drip header answered with %d, want an error or dropped connection", resp.StatusCode)
		}
	}
}

// Fast requests are unaffected — the ordinary sanity check on the shortened
// timeout, so the test above's failure mode is "silent 200" rather than "the
// timeout was already too short for a normal request".
func TestReadHeaderTimeoutLeavesFastRequestsAlone(t *testing.T) {
	t.Cleanup(server.SetReadHeaderTimeoutForTest(150 * time.Millisecond))

	h := chartrtest.Start(t)
	if code, _ := h.Get("/api/health"); code != 200 {
		t.Fatalf("fast /api/health = %d, want 200", code)
	}
}
