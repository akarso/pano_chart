package http

import (
	"fmt"
	"net"
	"strings"
)

// DefaultDebugAddr is the loopback listener for debug routes.
// It is a different socket from the public API.
const DefaultDebugAddr = "127.0.0.1:8082"

// LoopbackListenAddr returns addr when its host is a loopback IP.
// An empty addr uses DefaultDebugAddr. A missing host (`:8082`), a wildcard,
// or any public IP is rejected so the debug server cannot bind where a
// reverse proxy publishes the API.
func LoopbackListenAddr(addr string) (string, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return DefaultDebugAddr, nil
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("debug listen address: %w", err)
	}
	if port == "" {
		return "", fmt.Errorf("debug listen address missing port")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return "", fmt.Errorf("debug listen address must be a loopback IP, got %q", host)
	}
	return net.JoinHostPort(ip.String(), port), nil
}
