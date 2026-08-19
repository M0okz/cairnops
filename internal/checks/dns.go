package checks

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/M0okz/cairnops/internal/domain"
)

type DNS struct{}

type DNSConfig struct {
	Name     string   `json:"name"`
	Type     string   `json:"type,omitempty"`
	Server   string   `json:"server,omitempty"`
	Expected []string `json:"expected,omitempty"`
}

func (DNS) Check(ctx context.Context, source domain.Source) domain.Observation {
	startedAt := time.Now()
	var config DNSConfig
	if err := decodeConfig(source.Config, &config); err != nil {
		return unknown(source, startedAt, "invalid_config", err)
	}
	if strings.TrimSpace(config.Name) == "" {
		return unknown(source, startedAt, "invalid_config", fmt.Errorf("name is required"))
	}
	queryType := strings.ToUpper(config.Type)
	if queryType == "" {
		queryType = "A"
	}
	if !supportedDNSQueryType(queryType) {
		return unknown(source, startedAt, "invalid_config", fmt.Errorf("unsupported DNS query type %q", queryType))
	}
	resolver, err := dnsResolver(config.Server, source.Timeout)
	if err != nil {
		return unknown(source, startedAt, "invalid_config", err)
	}

	values, err := lookupDNS(ctx, resolver, queryType, config.Name)
	if err != nil {
		return unhealthy(source, startedAt, "dns_lookup_failed", err)
	}
	sort.Strings(values)
	observation := healthy(source, startedAt)
	observation.Details["query_type"] = queryType
	observation.Details["answers"] = values
	if len(values) == 0 {
		observation.Outcome = domain.OutcomeUnhealthy
		observation.Reason = "dns_empty_answer"
		observation.Message = "DNS response contains no answer"
		return observation
	}
	for _, expected := range config.Expected {
		if !containsFold(values, expected) {
			observation.Outcome = domain.OutcomeUnhealthy
			observation.Reason = "dns_assertion_failed"
			observation.Message = fmt.Sprintf("expected DNS value %q was not found", expected)
			break
		}
	}
	return observation
}

func dnsResolver(server string, timeout time.Duration) (*net.Resolver, error) {
	if server == "" {
		return net.DefaultResolver, nil
	}
	if _, _, err := net.SplitHostPort(server); err != nil {
		return nil, fmt.Errorf("DNS server must contain host and port: %w", err)
	}
	dialer := &net.Dialer{Timeout: timeout}
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "udp", server)
		},
	}, nil
}

func lookupDNS(ctx context.Context, resolver *net.Resolver, queryType, name string) ([]string, error) {
	switch queryType {
	case "A", "AAAA":
		network := "ip4"
		if queryType == "AAAA" {
			network = "ip6"
		}
		addresses, err := resolver.LookupIP(ctx, network, name)
		if err != nil {
			return nil, err
		}
		values := make([]string, 0, len(addresses))
		for _, address := range addresses {
			values = append(values, address.String())
		}
		return values, nil
	case "CNAME":
		value, err := resolver.LookupCNAME(ctx, name)
		return []string{strings.TrimSuffix(value, ".")}, err
	case "MX":
		records, err := resolver.LookupMX(ctx, name)
		values := make([]string, 0, len(records))
		for _, record := range records {
			values = append(values, fmt.Sprintf("%d %s", record.Pref, strings.TrimSuffix(record.Host, ".")))
		}
		return values, err
	case "TXT":
		return resolver.LookupTXT(ctx, name)
	case "NS":
		records, err := resolver.LookupNS(ctx, name)
		values := make([]string, 0, len(records))
		for _, record := range records {
			values = append(values, strings.TrimSuffix(record.Host, "."))
		}
		return values, err
	case "SRV":
		_, records, err := resolver.LookupSRV(ctx, "", "", name)
		values := make([]string, 0, len(records))
		for _, record := range records {
			values = append(values, fmt.Sprintf("%d %d %d %s", record.Priority, record.Weight, record.Port, strings.TrimSuffix(record.Target, ".")))
		}
		return values, err
	case "PTR":
		return resolver.LookupAddr(ctx, name)
	default:
		return nil, fmt.Errorf("unsupported DNS query type %q", queryType)
	}
}

func containsFold(values []string, expected string) bool {
	for _, value := range values {
		if strings.EqualFold(value, expected) {
			return true
		}
	}
	return false
}

func supportedDNSQueryType(queryType string) bool {
	switch queryType {
	case "A", "AAAA", "CNAME", "MX", "TXT", "NS", "SRV", "PTR":
		return true
	default:
		return false
	}
}
