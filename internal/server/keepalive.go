package server

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// pingInterval bounds idle websocket connections from being cut by reverse
// proxies (docker-deploy, ticket 03). The server fires a ping frame every
// interval on the control and terminal sockets; coder/websocket responds to
// pings automatically at the peer, so the ping is purely a keepalive.
// Package-scoped so a test can shorten it via SetPingIntervalForTest.
var pingInterval = 30 * time.Second

// pingsSent counts server-issued ping attempts across all sockets. It is a
// test observable — the ping frames themselves are protocol-level and the
// client library handles them without surfacing them to Read. Production
// code never reads it.
var pingsSent atomic.Int64

// SetPingIntervalForTest lowers the keepalive interval for the duration of a
// test and returns a restore function. Exported for tests in a sibling
// package that would otherwise wait 30s to see a ping.
func SetPingIntervalForTest(d time.Duration) func() {
	prev := pingInterval
	pingInterval = d
	return func() { pingInterval = prev }
}

// PingsSentForTest returns the current ping-attempt count.
func PingsSentForTest() int64 { return pingsSent.Load() }

// keepalive fires pings at pingInterval until ctx ends. It runs on its own
// goroutine and returns when the context is cancelled — cancelling ctx (as
// the handlers do on close) is how the caller ends this loop.
func keepalive(ctx context.Context, c *websocket.Conn) {
	if pingInterval <= 0 {
		return
	}
	t := time.NewTicker(pingInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pingsSent.Add(1)
			// A short bounded context on Ping: a wedged peer must not pin the
			// keepalive goroutine, and coder/websocket returns whatever pong
			// arrives (or an error) within the deadline. On error we let the
			// handler notice the closed socket the way it does today rather
			// than tearing it down from here.
			pingCtx, cancel := context.WithTimeout(ctx, pingInterval)
			_ = c.Ping(pingCtx)
			cancel()
		}
	}
}
