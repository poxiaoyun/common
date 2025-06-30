package net

import (
	"net"
	"strings"
)

// SplitHostPort splits a host:port string into host and port components.
// It unlike the standard library's [net.SplitHostPort] in that it does not
// return an error if the input does not contain a port. Instead, it returns
// the entire string as the host and an empty string as the port.
// It also handles IPv6 addresses by removing the square brackets
// that may surround the address.
func SplitHostPort(hostport string) (host, port string) {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		return hostport, ""
	}
	// If the host is an IPv6 address, it may be enclosed in square brackets.
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	return host, port
}
