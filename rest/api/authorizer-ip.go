package api

import (
	"net"
	"net/http"
)

func NewAllowCIDRAuthorizer(cidrs []string, defaultDec Decision) RequestAuthorizer {
	return RequestAuthorizerFunc(func(r *http.Request) (Decision, string, error) {
		if RequestSourceIPInCIDR(cidrs, r) {
			return DecisionAllow, "", nil
		}
		return defaultDec, "", nil
	})
}

func RequestSourceIPInCIDR(cidrs []string, r *http.Request) bool {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return InCIDR(ip, cidrs)
}

func InCIDR(ip string, cidrs []string) bool {
	for _, cidr := range cidrs {
		if cidr == ip {
			return true
		}
		// check if ip is in cidr
		_, ipn, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if ipn.Contains(net.ParseIP(ip)) {
			return true
		}
	}
	return false
}
