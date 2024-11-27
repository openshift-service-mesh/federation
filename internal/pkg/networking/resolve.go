package networking

import (
	"fmt"
	"net"

	"istio.io/istio/pkg/slices"
)

func Resolve(addr string) []string {
	if ip := net.ParseIP(addr); ip != nil {
		return []string{addr}
	}

	ips, err := net.LookupIP(addr)
	if err != nil {
		fmt.Printf("Failed to resolve '%s': %v\n", addr, err)
	}
	return slices.Map(ips, func(ip net.IP) string {
		return ip.String()
	})
}
