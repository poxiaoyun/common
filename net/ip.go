package net

import (
	"fmt"
	"math"
	"math/big"
	"net"
)

type IPFamily string

const (
	IPv4            IPFamily = "IPv4"
	IPv6            IPFamily = "IPv6"
	IPFamilyUnknown IPFamily = "Unknown"
)

// IPFamilyOf returns the IP family of ip, or IPFamilyUnknown if it is invalid.
func IPFamilyOf(ip net.IP) IPFamily {
	switch {
	case ip.To4() != nil:
		return IPv4
	case ip.To16() != nil:
		return IPv4
	default:
		return IPFamilyUnknown
	}
}

// IsIPv6 returns true if netIP is IPv6 (and false if it is IPv4, nil, or invalid).
func IsIPv6(netIP net.IP) bool {
	return IPFamilyOf(netIP) == IPv6
}

func IsIPv4(netIP net.IP) bool {
	return IPFamilyOf(netIP) == IPv4
}

// RangeSize returns the size of a range in valid addresses.
// returns the size of the subnet (or math.MaxInt64 if the range size would overflow int64)
func RangeSize(subnet *net.IPNet) int64 {
	ones, bits := subnet.Mask.Size()
	if bits == 32 && (bits-ones) >= 31 || bits == 128 && (bits-ones) >= 127 {
		return 0
	}
	if bits-ones >= 63 {
		return math.MaxInt64
	}
	return int64(1) << uint(bits-ones)
}

// AddIPOffset adds the provided integer offset to a base big.Int representing a net.IP
// NOTE: If you started with a v4 address and overflow it, you get a v6 result.
func AddIPOffset(base *big.Int, offset int) net.IP {
	r := big.NewInt(0).Add(base, big.NewInt(int64(offset))).Bytes()
	r = append(make([]byte, 16), r...)
	return net.IP(r[len(r)-16:])
}

// GetIndexedIP returns a net.IP that is subnet.IP + index in the contiguous IP space.
func GetIndexedIP(subnet *net.IPNet, index int) (net.IP, error) {
	ip := AddIPOffset(BigForIP(subnet.IP), index)
	if !subnet.Contains(ip) {
		return nil, fmt.Errorf("can't generate IP with index %d from subnet. subnet too small. subnet: %q", index, subnet)
	}
	return ip, nil
}

// BigForIP creates a big.Int based on the provided net.IP
func BigForIP(ip net.IP) *big.Int {
	return big.NewInt(0).SetBytes(ip.To16())
}

func ParsePort(port string, allowZero bool) (int, error) {
	portint, err := net.LookupPort("", port)
	if err != nil {
		return 0, err
	}
	if portint == 0 && !allowZero {
		return 0, net.InvalidAddrError("0 is not a valid port number")
	}
	return portint, nil
}
