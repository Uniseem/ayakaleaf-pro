package realtime

import (
	"net"
	"net/http"
	"strings"
)

// AddressResolver works out a client's real IP address behind a proxy.
//
// It reproduces what the proxy-addr package does for Express: start at the
// socket address and walk X-Forwarded-For from right to left for as long as
// the address in hand is a proxy we trust. Trusting the header outright would
// let any client claim any address, which matters here because the address is
// logged and rate-limited on.
type AddressResolver struct {
	behindProxy bool
	// nets are the trusted proxy ranges; nil means trust nothing.
	nets []*net.IPNet
}

// loopbackRanges is what the "loopback" shorthand expands to, matching
// proxy-addr's own definition.
var loopbackRanges = []string{"127.0.0.0/8", "::1/128"}

// NewAddressResolver compiles a trusted-proxy list, accepting the same
// comma-separated CIDR/address syntax as TRUSTED_PROXY_IPS.
func NewAddressResolver(behindProxy bool, trusted string) *AddressResolver {
	r := &AddressResolver{behindProxy: behindProxy}
	if !behindProxy {
		return r
	}
	for _, entry := range strings.Split(trusted, ",") {
		entry = strings.TrimSpace(entry)
		switch entry {
		case "":
			continue
		case "loopback":
			for _, cidr := range loopbackRanges {
				if _, n, err := net.ParseCIDR(cidr); err == nil {
					r.nets = append(r.nets, n)
				}
			}
		default:
			if _, n, err := net.ParseCIDR(entry); err == nil {
				r.nets = append(r.nets, n)
				continue
			}
			// A bare address is a /32 or /128.
			if ip := net.ParseIP(entry); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				r.nets = append(r.nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			}
		}
	}
	return r
}

func (r *AddressResolver) trusts(ip net.IP) bool {
	for _, n := range r.nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// RemoteIP resolves the address of the client that made a request.
func (r *AddressResolver) RemoteIP(req *http.Request) string {
	if req == nil {
		return "client-handshake-missing"
	}
	addr := req.RemoteAddr
	if host, _, err := net.SplitHostPort(addr); err == nil {
		addr = host
	}
	if !r.behindProxy || len(r.nets) == 0 {
		return addr
	}

	forwarded := req.Header.Get("X-Forwarded-For")
	if forwarded == "" {
		return addr
	}
	hops := strings.Split(forwarded, ",")
	current := addr
	for i := len(hops) - 1; i >= 0; i-- {
		ip := net.ParseIP(current)
		if ip == nil || !r.trusts(ip) {
			// The address in hand did not come from a proxy we trust, so
			// everything to its left in the header is unverifiable.
			return current
		}
		next := strings.TrimSpace(hops[i])
		if next == "" {
			return current
		}
		current = next
	}
	return current
}
