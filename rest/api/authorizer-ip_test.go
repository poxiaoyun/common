package api

import (
	"net/http/httptest"
	"testing"
)

func TestRequestSourceIPInCIDR(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		cidrs      []string
		want       bool
	}{
		{name: "IPv4", remoteAddr: "192.0.2.10:4321", cidrs: []string{"192.0.2.0/24"}, want: true},
		{name: "IPv6", remoteAddr: "[2001:db8::10]:4321", cidrs: []string{"2001:db8::/32"}, want: true},
		{name: "invalid remote address", remoteAddr: "192.0.2.10", cidrs: []string{"192.0.2.0/24"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/", nil)
			request.RemoteAddr = tt.remoteAddr
			if got := RequestSourceIPInCIDR(tt.cidrs, request); got != tt.want {
				t.Fatalf("RequestSourceIPInCIDR() = %t, want %t", got, tt.want)
			}
		})
	}
}
