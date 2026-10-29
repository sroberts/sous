package safefetch

import (
	"errors"
	"net/netip"
	"testing"
)

func TestPublic(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"93.184.215.14", true},
		{"2606:4700:4700::1111", true},
		{"::ffff:8.8.8.8", true},

		{"0.0.0.0", false},
		{"0.1.2.3", false},
		{"10.0.0.1", false},
		{"127.0.0.1", false},
		{"127.255.255.254", false},
		{"172.15.255.255", true},
		{"172.16.0.1", false},
		{"172.31.255.255", false},
		{"172.32.0.0", true},
		{"192.168.1.1", false},
		{"169.254.169.254", false},
		{"169.254.0.1", false},
		{"224.0.0.1", false},
		{"239.255.255.250", false},
		{"240.0.0.1", false},
		{"255.255.255.255", false},
		{"192.0.0.8", false},
		{"192.0.2.1", false},
		{"198.18.0.1", false},
		{"198.19.255.255", false},
		{"198.20.0.0", true},
		{"198.51.100.1", false},
		{"203.0.113.5", false},

		// CGNAT, which is also the tailnet range.
		{"100.63.255.255", true},
		{"100.64.0.0", false},
		{"100.100.100.100", false},
		{"100.127.255.255", false},
		{"100.128.0.0", true},

		{"::", false},
		{"::1", false},
		{"::ffff:127.0.0.1", false},
		{"::ffff:10.0.0.1", false},
		{"::ffff:169.254.169.254", false},
		{"::ffff:100.64.0.1", false},
		{"::127.0.0.1", false}, // IPv4-compatible
		{"::8.8.8.8", false},
		{"::ffff:0:10.0.0.1", false}, // IPv4-translated
		{"fe80::1", false},
		{"fe80::1%en0", false},
		{"fec0::1", false},
		{"fc00::1", false},
		{"fd7a:115c:a1e0::1", false}, // tailnet v6
		{"ff02::1", false},
		{"64:ff9b::a00:1", false},   // NAT64 of 10.0.0.1
		{"64:ff9b::808:808", false}, // NAT64 is blocked outright
		{"64:ff9b:1::1", false},
		{"2002:a00:1::1", false}, // 6to4 of 10.0.0.1
		{"2001::1", false},       // Teredo
		{"2001:db8::1", false},
		{"100::1", false},
	}
	for _, tc := range cases {
		a := netip.MustParseAddr(tc.addr)
		if got := Public(a); got != tc.want {
			t.Errorf("Public(%s) = %v, want %v", tc.addr, got, tc.want)
		}
	}
	if Public(netip.Addr{}) {
		t.Error("Public(zero Addr) = true")
	}
}

func TestControlBackstop(t *testing.T) {
	c := New(Options{})
	for _, addr := range []string{
		"10.0.0.1:80",
		"127.0.0.1:443",
		"[::1]:80",
		"[::ffff:127.0.0.1]:80",
		"169.254.169.254:80",
		"100.64.0.1:80",
		"[64:ff9b::7f00:1]:80",
	} {
		err := c.control("tcp", addr, nil)
		var be *BlockedError
		if !errors.As(err, &be) {
			t.Errorf("control(%s) = %v, want BlockedError", addr, err)
		}
	}
	if err := c.control("tcp", "8.8.8.8:443", nil); err != nil {
		t.Errorf("control(8.8.8.8:443) = %v", err)
	}
	if err := c.control("tcp", "garbage", nil); err == nil {
		t.Error("control(garbage) = nil")
	}
}

func TestOwnInterfaceAddrsBlocked(t *testing.T) {
	c := New(Options{})
	if !c.self[netip.MustParseAddr("127.0.0.1")] {
		t.Error("interface addresses missing loopback 127.0.0.1")
	}
	pub := netip.MustParseAddr("8.8.8.8")
	if !c.permitted(netip.AddrPortFrom(pub, 443)) {
		t.Fatal("8.8.8.8 should be permitted")
	}
	c.self[pub] = true
	if c.permitted(netip.AddrPortFrom(pub, 443)) {
		t.Error("own interface address permitted")
	}
	if err := c.control("tcp", "8.8.8.8:443", nil); err == nil {
		t.Error("control let an own interface address through")
	}
}

func TestAllowAddrPortsIsExact(t *testing.T) {
	ap := netip.MustParseAddrPort("127.0.0.1:8080")
	c := New(Options{Testing: &TestHooks{AllowAddrPorts: []netip.AddrPort{ap}}})
	if !c.permitted(ap) {
		t.Error("allowlisted addr:port refused")
	}
	if !c.permitted(netip.MustParseAddrPort("[::ffff:127.0.0.1]:8080")) {
		t.Error("mapped form of allowlisted addr:port refused")
	}
	if c.permitted(netip.MustParseAddrPort("127.0.0.1:8081")) {
		t.Error("other port on allowlisted address permitted")
	}
}
