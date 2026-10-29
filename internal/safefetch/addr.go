package safefetch

import (
	"fmt"
	"net"
	"net/netip"
)

// blockedPrefixes are ranges that IsGlobalUnicast and IsPrivate let through
// but that must never be fetched: shared, reserved, documentation, benchmark
// and translation ranges (NAT64, 6to4 and Teredo can tunnel to private v4).
var blockedPrefixes = func() []netip.Prefix {
	var ps []netip.Prefix
	for _, s := range []string{
		"0.0.0.0/8",
		"100.64.0.0/10",
		"192.0.0.0/24",
		"192.0.2.0/24",
		"198.18.0.0/15",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"240.0.0.0/4",
		"255.255.255.255/32",
		"2001:db8::/32",
		"64:ff9b::/96",
		"64:ff9b:1::/48",
		"2002::/16",
		"2001::/32",
		"fec0::/10",
	} {
		ps = append(ps, netip.MustParsePrefix(s))
	}
	return ps
}()

// globalV6 is the only IPv6 space allocated for global unicast. Requiring it
// also rejects IPv4-compatible (::a.b.c.d) and IPv4-translated forms.
var globalV6 = netip.MustParsePrefix("2000::/3")

// Public reports whether a is a publicly routable unicast address that Sous
// may fetch from. IPv4-mapped IPv6 addresses are judged as IPv4.
func Public(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsValid() || a.Zone() != "" {
		return false
	}
	if !a.IsGlobalUnicast() || a.IsPrivate() {
		return false
	}
	if a.Is6() && !globalV6.Contains(a) {
		return false
	}
	for _, p := range blockedPrefixes {
		if p.Contains(a) {
			return false
		}
	}
	return true
}

// BlockedError is returned when a host resolves to an address Sous refuses to
// connect to.
type BlockedError struct {
	Host string
	Addr netip.Addr
}

func (e *BlockedError) Error() string {
	if e.Host == "" || e.Host == e.Addr.String() {
		return fmt.Sprintf("safefetch: address %s is not public", e.Addr)
	}
	return fmt.Sprintf("safefetch: %s resolves to non-public address %s", e.Host, e.Addr)
}

// interfaceAddrs returns the host's own addresses, so a public address bound
// to this machine can't be used to reach its services.
func interfaceAddrs() map[netip.Addr]bool {
	self := map[netip.Addr]bool{}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return self
	}
	for _, a := range addrs {
		var ip net.IP
		switch v := a.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if na, ok := netip.AddrFromSlice(ip); ok {
			self[na.Unmap()] = true
		}
	}
	return self
}
