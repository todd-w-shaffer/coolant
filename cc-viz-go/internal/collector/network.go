package collector

import (
	"net"
	"time"
)

// CheckOnline tests network reachability by attempting a TCP connection
// to the Claude API endpoint. Fast timeout — we just need to know if
// packets can leave the machine.
func CheckOnline() bool {
	conn, err := net.DialTimeout("tcp", "api.anthropic.com:443", 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
