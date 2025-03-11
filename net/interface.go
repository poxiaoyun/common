package net

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

type Interface struct {
	net.Interface
	Addrs []net.IPNet
}

// ListInterfaces returns a list of network interfaces that match the given
// include and exclude regular expressions. If includeRegexes is empty.
//
//	version is 4 or 6.
func ListInterfaces(includeRegexes []string, excludeRegexes []string, version int) ([]Interface, error) {
	netIfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var ret []Interface
	var includeRegexp *regexp.Regexp
	var excludeRegexp *regexp.Regexp
	if len(includeRegexes) > 0 {
		if includeRegexp, err = regexp.Compile("(" + strings.Join(includeRegexes, ")|(") + ")"); err != nil {
			return nil, err
		}
	}
	if len(excludeRegexes) > 0 {
		if excludeRegexp, err = regexp.Compile("(" + strings.Join(excludeRegexes, ")|(") + ")"); err != nil {
			return nil, err
		}
	}
	for _, iface := range netIfaces {
		include := (includeRegexp == nil) || includeRegexp.MatchString(iface.Name)
		exclude := (excludeRegexp != nil) && excludeRegexp.MatchString(iface.Name)
		if !include || exclude {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, fmt.Errorf("Failed to get addresses for interface %s: %v", iface.Name, err)
		}
		inf := Interface{
			Interface: iface,
		}
		for _, addr := range addrs {
			switch val := addr.(type) {
			case *net.IPNet:
				if version == 0 || GetVersion(val.IP) == version {
					inf.Addrs = append(inf.Addrs, *val)
				}
			case *net.IPAddr:
				if version == 0 || GetVersion(val.IP) == version {
					inf.Addrs = append(inf.Addrs, net.IPNet{IP: val.IP})
				}
			default:
			}
		}
		ret = append(ret, inf)
	}
	return ret, nil
}

func GetVersion(ip net.IP) int {
	if ip.To4() != nil {
		return 4
	}
	if len(ip) == net.IPv6len {
		return 6
	}
	return 0
}
