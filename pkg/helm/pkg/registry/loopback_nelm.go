package registry

import (
	"net"
	"strings"
)

// IsLoopbackHost returns whether the registry host (host or host:port) is a loopback one, which
// is reached over plain HTTP by default, as containerd and go-containerregistry do.
func IsLoopbackHost(host string) bool {
	hostname := host
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		hostname = splitHost
	}

	hostname = strings.TrimSuffix(strings.TrimPrefix(hostname, "["), "]")
	if hostname == "localhost" {
		return true
	}

	ip := net.ParseIP(hostname)

	return ip != nil && ip.IsLoopback()
}
