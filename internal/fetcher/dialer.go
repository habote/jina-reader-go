package fetcher

import (
	"errors"
	"net"
	"syscall"
	"time"
)

var (
	ErrSSRFForbidden = errors.New("access to private or loopback IP is forbidden")
	ErrInvalidScheme = errors.New("only http and https schemes are allowed")
	ErrBodyTooLarge  = errors.New("response body exceeded maximum allowed bytes")
)

// Private and reserved IP blocks according to RFC standards.
var privateIPBlocks []*net.IPNet

func init() {
	cidrs := []string{
		"127.0.0.0/8",    // Loopback IPv4
		"::1/128",        // Loopback IPv6
		"10.0.0.0/8",     // Private IPv4 (RFC 1918)
		"172.16.0.0/12",  // Private IPv4 (RFC 1918)
		"192.168.0.0/16", // Private IPv4 (RFC 1918)
		"169.254.0.0/16", // Link-local IPv4
		"fe80::/10",      // Link-local IPv6
		"100.64.0.0/10",  // Carrier-grade NAT (RFC 6598)
		"0.0.0.0/8",      // Current network (RFC 1122)
		"fc00::/7",       // Unique local IPv6 (RFC 4193)
	}

	for _, cidr := range cidrs {
		_, block, err := net.ParseCIDR(cidr)
		if err == nil {
			privateIPBlocks = append(privateIPBlocks, block)
		}
	}
}

// IsPrivateIP checks if an IP belongs to private, loopback, link-local, multicast, or unspecified ranges.
func IsPrivateIP(ip net.IP) bool {
	if ip == nil {
		return true
	}

	// Normalise IPv4-mapped IPv6 addresses (::ffff:127.0.0.1)
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}

	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}

	for _, block := range privateIPBlocks {
		if block.Contains(ip) {
			return true
		}
	}

	return false
}

type DialerConfig struct {
	AllowLocalhost bool // true for local unit testing
	Timeout        time.Duration
}

// NewSafeDialer creates a net.Dialer with custom Control callback preventing connections to private/loopback IPs.
func NewSafeDialer(cfg DialerConfig) *net.Dialer {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			if cfg.AllowLocalhost {
				return nil
			}

			host, _, err := net.SplitHostPort(address)
			if err != nil {
				host = address
			}

			ip := net.ParseIP(host)
			if ip != nil && IsPrivateIP(ip) {
				return ErrSSRFForbidden
			}

			return nil
		},
	}
}
