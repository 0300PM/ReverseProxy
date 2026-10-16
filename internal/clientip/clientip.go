package clientip

import (
	"net"
	"net/http"
	"strings"
)

type Resolver struct {
	trusted []*net.IPNet
}

func New(trustedProxies []string) (*Resolver, error) {
	r := &Resolver{}
	for _, raw := range trustedProxies {
		if ip := net.ParseIP(raw); ip != nil {
			r.trusted = append(r.trusted, ipToNet(ip))
			continue
		}
		_, network, err := net.ParseCIDR(raw)
		if err != nil {
			return nil, err
		}
		r.trusted = append(r.trusted, network)
	}
	return r, nil
}

func (r *Resolver) FromRequest(req *http.Request) string {
	host := req.RemoteAddr
	if h, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		host = h
	}
	remoteIP := net.ParseIP(host)
	if remoteIP == nil || !r.isTrusted(remoteIP) {
		return host
	}
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			ip := net.ParseIP(strings.TrimSpace(parts[i]))
			if ip == nil {
				break
			}
			if !r.isTrusted(ip) {
				return ip.String()
			}
		}
		if ip := net.ParseIP(strings.TrimSpace(parts[0])); ip != nil {
			return ip.String()
		}
		return host
	}
	if xri := req.Header.Get("X-Real-IP"); xri != "" {
		if ip := net.ParseIP(strings.TrimSpace(xri)); ip != nil {
			return ip.String()
		}
	}
	return host
}

func (r *Resolver) isTrusted(ip net.IP) bool {
	for _, n := range r.trusted {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func ipToNet(ip net.IP) *net.IPNet {
	if v4 := ip.To4(); v4 != nil {
		return &net.IPNet{IP: v4, Mask: net.CIDRMask(32, 32)}
	}
	return &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}
}
