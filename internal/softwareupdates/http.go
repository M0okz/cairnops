package softwareupdates

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
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

var errResponseTooLarge = errors.New("response too large")

// RateLimitedError signale qu'un hôte a demandé de suspendre les requêtes.
// Tant que la date n'est pas atteinte, aucune requête ne lui est envoyée :
// insister consommerait le quota anonyme partagé par tous les services.
type RateLimitedError struct {
	Host  string
	Until time.Time
}

func (e *RateLimitedError) Error() string { return "rate_limited:" + e.Host }

type hostLimits struct {
	mu    sync.Mutex
	until map[string]time.Time
}

var limits = hostLimits{until: map[string]time.Time{}}

func (l *hostLimits) blocked(host string, now time.Time) (time.Time, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	until, ok := l.until[host]
	if ok && !now.Before(until) {
		delete(l.until, host)
		return time.Time{}, false
	}
	return until, ok
}

func (l *hostLimits) block(host string, until time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if until.After(l.until[host]) {
		l.until[host] = until
	}
}

// rateLimitUntil lit les en-têtes standard et ceux de GitHub. Une réponse 403
// sans indication de quota reste un refus ordinaire.
func rateLimitUntil(res *http.Response, now time.Time) (time.Time, bool) {
	if res.StatusCode != http.StatusTooManyRequests && res.StatusCode != http.StatusForbidden {
		return time.Time{}, false
	}
	var until time.Time
	if value := strings.TrimSpace(res.Header.Get("Retry-After")); value != "" {
		if seconds, err := strconv.Atoi(value); err == nil {
			until = now.Add(time.Duration(seconds) * time.Second)
		} else if at, err := http.ParseTime(value); err == nil {
			until = at
		}
	}
	if until.IsZero() && res.Header.Get("X-RateLimit-Remaining") == "0" {
		if reset, err := strconv.ParseInt(res.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			until = time.Unix(reset, 0)
		} else {
			until = now.Add(time.Hour)
		}
	}
	if until.IsZero() {
		if res.StatusCode != http.StatusTooManyRequests {
			return time.Time{}, false
		}
		until = now.Add(time.Hour)
	}
	if until.Before(now.Add(time.Minute)) {
		until = now.Add(time.Minute)
	}
	if until.After(now.Add(24 * time.Hour)) {
		until = now.Add(24 * time.Hour)
	}
	return until, true
}

func request(ctx context.Context, client *http.Client, method, endpoint, key string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("invalid request")
	}
	host := strings.ToLower(req.URL.Hostname())
	if until, blocked := limits.blocked(host, time.Now()); blocked {
		return nil, &RateLimitedError{Host: host, Until: until}
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
		if until, limited := rateLimitUntil(res, time.Now()); limited {
			limits.block(host, until)
			return nil, &RateLimitedError{Host: host, Until: until}
		}
		return nil, fmt.Errorf("remote HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 2<<20+1))
	if err != nil {
		return nil, fmt.Errorf("read response failed")
	}
	if len(b) > 2<<20 {
		return nil, errResponseTooLarge
	}
	return b, nil
}
