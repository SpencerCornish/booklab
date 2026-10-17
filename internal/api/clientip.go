package api

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// clientIP returns the address of the client that sent r, for rate limiting and logs.
// X-Forwarded-For is only believed when the direct peer is a trusted proxy. Its entries
// are then read right to left (each proxy appends the address it received from) and the
// first one that is not itself a trusted proxy is the client. Anything left of that was
// written by the client and is ignored.
func (s *Server) clientIP(r *http.Request) string {
	peer, ok := parseHostAddr(r.RemoteAddr)
	if !ok {
		return r.RemoteAddr
	}
	if !s.isTrustedProxy(peer) {
		return peer.String()
	}

	var hops []string
	for _, v := range r.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(v, ",")...)
	}
	client := peer
	for i := len(hops) - 1; i >= 0; i-- {
		addr, ok := parseHostAddr(strings.TrimSpace(hops[i]))
		if !ok {
			break
		}
		client = addr
		if !s.isTrustedProxy(addr) {
			break
		}
	}
	return client.String()
}

func (s *Server) isTrustedProxy(addr netip.Addr) bool {
	for _, p := range s.trustedProxies {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// parseHostAddr parses an IP with or without a port, dropping any IPv6 zone and
// unmapping IPv4-mapped IPv6 addresses.
func parseHostAddr(s string) (netip.Addr, bool) {
	if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	}
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.WithZone("").Unmap(), true
}
