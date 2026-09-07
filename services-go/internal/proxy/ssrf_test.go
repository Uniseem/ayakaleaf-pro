package proxy

import (
	"net/netip"
	"testing"
)

func TestIsUnicastBlocksSSRFTargets(t *testing.T) {
	blocked := []string{
		"127.0.0.1",        // loopback
		"127.1.2.3",        // loopback, whole /8
		"10.0.0.1",         // private
		"172.16.5.4",       // private
		"192.168.1.1",      // private
		"169.254.169.254",  // cloud metadata endpoint
		"100.64.0.1",       // carrier-grade NAT
		"0.0.0.0",          // unspecified
		"255.255.255.255",  // broadcast
		"224.0.0.1",        // multicast
		"198.18.0.1",       // benchmarking
		"192.0.2.1",        // reserved (TEST-NET-1)
		"240.0.0.1",        // reserved
		"::1",              // IPv6 loopback
		"fe80::1",          // IPv6 link-local
		"fc00::1",          // IPv6 unique local
		"ff02::1",          // IPv6 multicast
		"::",               // IPv6 unspecified
		"2001:db8::1",      // IPv6 documentation
		"::ffff:127.0.0.1", // IPv4-mapped loopback
		"::ffff:10.0.0.1",  // IPv4-mapped private
	}
	for _, s := range blocked {
		addr := netip.MustParseAddr(s)
		if IsUnicast(addr) {
			t.Errorf("IsUnicast(%s) = true, want false", s)
		}
	}

	allowed := []string{"8.8.8.8", "1.1.1.1", "93.184.216.34", "2606:2800:220:1:248:1893:25c8:1946"}
	for _, s := range allowed {
		addr := netip.MustParseAddr(s)
		if !IsUnicast(addr) {
			t.Errorf("IsUnicast(%s) = false, want true", s)
		}
	}
}

func TestIsBlockedHonoursConfiguredNetworks(t *testing.T) {
	blocked, err := ParseBlockedNetworks([]string{"203.0.114.0/24", "2001:4860::/32"})
	if err != nil {
		t.Fatal(err)
	}
	if !IsBlocked(netip.MustParseAddr("203.0.114.7"), blocked) {
		t.Error("address inside a configured blocked network should be blocked")
	}
	if !IsBlocked(netip.MustParseAddr("2001:4860:4860::8888"), blocked) {
		t.Error("IPv6 address inside a configured blocked network should be blocked")
	}
	if IsBlocked(netip.MustParseAddr("8.8.8.8"), blocked) {
		t.Error("public address outside the blocked networks should be allowed")
	}
	// A v4 address must not be matched by a v6 prefix.
	if IsBlocked(netip.MustParseAddr("8.8.8.8"), mustParsePrefixes([]string{"::/0"})) {
		t.Error("v6 prefix should not match a v4 address")
	}
}

func TestParseBlockedNetworksRejectsGarbage(t *testing.T) {
	if _, err := ParseBlockedNetworks([]string{"not-a-cidr"}); err == nil {
		t.Error("expected an error for an unparseable CIDR")
	}
}
