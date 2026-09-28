package proxy

import (
	"fmt"
	"net/netip"
)

// nonUnicastV4 is ipaddr.js's IPv4 special-range table. Any address matching
// one of these has a range() other than "unicast", which the Node service
// treats as blocked.
//
// The security-relevant entries are loopback, the private blocks, link-local
// (169.254.169.254 is the cloud metadata endpoint) and carrier-grade NAT; the
// rest are carried over so the Go port is neither stricter nor looser than the
// implementation it replaces.
var nonUnicastV4 = []string{
	"0.0.0.0/8",          // unspecified
	"255.255.255.255/32", // broadcast
	"224.0.0.0/4",        // multicast
	"169.254.0.0/16",     // linkLocal
	"127.0.0.0/8",        // loopback
	"100.64.0.0/10",      // carrierGradeNat
	"10.0.0.0/8",         // private
	"172.16.0.0/12",      // private
	"192.168.0.0/16",     // private
	"192.0.0.0/24",       // reserved
	"192.0.2.0/24",       // reserved
	"192.88.99.0/24",     // reserved
	"198.51.100.0/24",    // reserved
	"203.0.113.0/24",     // reserved
	"240.0.0.0/4",        // reserved
	"198.18.0.0/15",      // benchmarking
	"192.52.193.0/24",    // amt
	"192.175.48.0/24",    // as112
}

// nonUnicastV6 is ipaddr.js's IPv6 special-range table. IPv4-mapped addresses
// are absent because they are unwrapped and re-checked as IPv4 first.
var nonUnicastV6 = []string{
	"::/128",          // unspecified
	"::1/128",         // loopback
	"fe80::/10",       // linkLocal
	"ff00::/8",        // multicast
	"fc00::/7",        // uniqueLocal
	"64:ff9b::/96",    // rfc6052
	"2002::/16",       // 6to4
	"2001::/32",       // teredo
	"2001:2::/48",     // benchmarking
	"2001:3::/32",     // amt
	"2001:4:112::/48", // as112v6
	"2001:10::/28",    // deprecated
	"2001:20::/28",    // orchid2
	"2001:db8::/32",   // reserved
	"0100::/64",       // droppedAsUnusedInterfaceID
}

var (
	prefixesV4 = mustParsePrefixes(nonUnicastV4)
	prefixesV6 = mustParsePrefixes(nonUnicastV6)
)

func mustParsePrefixes(cidrs []string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, cidr := range cidrs {
		p, err := netip.ParsePrefix(cidr)
		if err != nil {
			panic("proxy: bad built-in CIDR " + cidr + ": " + err.Error())
		}
		out = append(out, p)
	}
	return out
}

// ParseBlockedNetworks parses the OVERLEAF_LINKED_URL_BLOCKED_NETWORKS entries.
// An unparseable entry is reported so the caller can answer 500, as the Node
// service does.
func ParseBlockedNetworks(cidrs []string) ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, cidr := range cidrs {
		p, err := netip.ParsePrefix(cidr)
		if err != nil {
			return nil, fmt.Errorf("Invalid blockedNetworks entry: %s", cidr)
		}
		out = append(out, p)
	}
	return out, nil
}

// IsUnicast reports whether addr falls outside every special range, which is
// what ipaddr.js's range() === "unicast" means.
func IsUnicast(addr netip.Addr) bool {
	addr = addr.Unmap()
	prefixes := prefixesV6
	if addr.Is4() {
		prefixes = prefixesV4
	}
	for _, p := range prefixes {
		if p.Contains(addr) {
			return false
		}
	}
	return true
}

// IsBlocked reports whether an address must not be fetched: either it is not a
// unicast address, or it falls inside an operator-configured blocked network.
func IsBlocked(addr netip.Addr, blocked []netip.Prefix) bool {
	addr = addr.Unmap()
	if !IsUnicast(addr) {
		return true
	}
	for _, p := range blocked {
		// A v4 address never matches a v6 prefix, and vice versa; Contains
		// already enforces that.
		if p.Contains(addr) {
			return true
		}
	}
	return false
}
