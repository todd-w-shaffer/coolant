package collector

import (
	"context"
	"net"
	"time"
)

// CheckOnline tests network reachability by attempting a TCP connection
// to the Claude API endpoint. Uses a context-aware dialer so the timeout
// covers DNS resolution too, not just the TCP handshake.
func CheckOnline(ctx context.Context) bool {
	d := net.Dialer{Timeout: 2 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", "api.anthropic.com:443")
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
