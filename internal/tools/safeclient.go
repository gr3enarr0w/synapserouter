package tools

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// ssrfSafeClient is an HTTP client that blocks requests to private/internal
// IP addresses to prevent SSRF attacks. Used by web_fetch and web_search tools.
var ssrfSafeClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		DialContext: ssrfSafeDialer,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	},
}

// ssrfSafeDialer resolves the target hostname and rejects private/internal IPs.
func ssrfSafeDialer(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}

	for _, ip := range ips {
		if isPrivateIP(ip.IP) {
			return nil, fmt.Errorf("blocked request to private/internal IP: %s (%s)", host, ip.IP)
		}
	}

	// Dial using the resolved (validated) address
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
}

// isPrivateIP returns true if the IP is in a private, loopback, or link-local range.
func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified()
}
