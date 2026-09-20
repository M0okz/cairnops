package softwareupdates

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

func publicURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(raw) > 2048 {
		return nil, fmt.Errorf("%w: public HTTPS URL required", ErrInvalid)
	}
	host := strings.ToLower(u.Hostname())
	if !strings.Contains(host, ".") || strings.HasSuffix(host, ".int.homeblack.fr") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".localhost") {
		return nil, fmt.Errorf("%w: public host required", ErrInvalid)
	}
	if ip, err := netip.ParseAddr(host); err == nil && !publicIP(ip) {
		return nil, fmt.Errorf("%w: public host required", ErrInvalid)
	}
	if u.Port() != "" && u.Port() != "443" {
		return nil, fmt.Errorf("%w: HTTPS port 443 required", ErrInvalid)
	}
	return u, nil
}
func publicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, s := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "64:ff9b::/96", "2002::/16"} {
		if netip.MustParsePrefix(s).Contains(ip) {
			return false
		}
	}
	return true
}
func publicClient() *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil
	tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("public host lookup failed")
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("public host has no address")
		}
		for _, ip := range ips {
			if !publicIP(ip) {
				return nil, fmt.Errorf("public host required")
			}
		}
		d := net.Dialer{Timeout: 10 * time.Second}
		return d.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}
	tr.ResponseHeaderTimeout = 20 * time.Second
	return &http.Client{Transport: tr, Timeout: 90 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
}
func request(ctx context.Context, client *http.Client, method, endpoint, key string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("invalid request")
	}
	req.Header.Set("User-Agent", "CairnOps-Version-Insights")
	req.Header.Set("Accept", "application/json, text/html, text/plain")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("remote request failed")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("remote HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 2<<20+1))
	if err != nil {
		return nil, fmt.Errorf("read response failed")
	}
	if len(b) > 2<<20 {
		return nil, fmt.Errorf("response too large")
	}
	return b, nil
}
