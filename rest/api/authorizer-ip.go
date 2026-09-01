package api

import (
	"net"
	"net/http"
	"slices"

	"xiaoshiai.cn/common/authz"
)

// NewAllowCIDRAuthorizer returns a request authorizer that allows matching
// source addresses and otherwise returns defaultDecision.
func NewAllowCIDRAuthorizer(cidrs []string, defaultDecision authz.Decision) RequestAuthorizerFunc {
	return func(r *http.Request) (authz.EvaluationResult, error) {
		if RequestSourceIPInCIDR(cidrs, r) {
			return authz.EvaluationResult{Decision: authz.DecisionAllow}, nil
		}
		return authz.EvaluationResult{Decision: defaultDecision}, nil
	}
}

func RequestSourceIPInCIDR(cidrs []string, r *http.Request) bool {
	if slices.Contains(cidrs, "*") {
		return true
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	return InCIDR(ip, cidrs)
}

func InCIDR(ip string, cidrs []string) bool {
	for _, cidr := range cidrs {
		if cidr == "*" {
			return true
		}
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
