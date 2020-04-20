package server

import (
	"fmt"
	"net"
	"strings"
)

// DefaultServerIPs is a default list of IPs server will bind to if nothing else is specified
var DefaultServerIPs = MultiIPs{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}

// ListenConfig is a wrapper around mutliple IPs and Port to bind to
type ListenConfig struct {
	IPs            MultiIPs
	Port           int
	ShouldAnnounce bool
	Iface          string
}

// MultiIPs is a wrapper allowing to set multiple IPs
type MultiIPs []net.IP

// Set adds check to the runlist
func (m *MultiIPs) Set(ipaddr string) error {
	ip := net.ParseIP(ipaddr)
	if ip == nil {
		return fmt.Errorf("invalid ip address %s", ip)
	}
	*m = append([]net.IP(*m), ip)
	return nil
}

// String returns joined list of checks
func (m *MultiIPs) String() string {
	ips := []string{}
	for _, ip := range *m {
		ips = append(ips, ip.String())
	}
	return strings.Join(ips, ", ")
}

// SetDefault adds all checks to the runlist
func (m *MultiIPs) SetDefault() {
	if len(*m) != 0 {
		return
	}

	*m = DefaultServerIPs
}
