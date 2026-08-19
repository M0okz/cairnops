package checks

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/M0okz/cairnops/internal/domain"
)

func ValidateConfig(kind domain.SourceKind, raw json.RawMessage) error {
	switch kind {
	case domain.SourceHTTP:
		var config HTTPConfig
		if err := decodeConfig(raw, &config); err != nil {
			return err
		}
		parsed, err := url.ParseRequestURI(config.URL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return fmt.Errorf("url must be an absolute HTTP or HTTPS URL")
		}
		method := strings.ToUpper(config.Method)
		if method != "" && method != http.MethodGet && method != http.MethodHead && method != http.MethodPost {
			return fmt.Errorf("method must be GET, HEAD, or POST")
		}
		for _, status := range config.AcceptedStatuses {
			if status < 100 || status > 599 {
				return fmt.Errorf("accepted HTTP statuses must be between 100 and 599")
			}
		}
		return nil
	case domain.SourceTCP:
		var config TCPConfig
		if err := decodeConfig(raw, &config); err != nil {
			return err
		}
		if _, _, err := net.SplitHostPort(config.Address); err != nil {
			return fmt.Errorf("address must contain host and port: %w", err)
		}
		return nil
	case domain.SourceDNS:
		var config DNSConfig
		if err := decodeConfig(raw, &config); err != nil {
			return err
		}
		if strings.TrimSpace(config.Name) == "" {
			return fmt.Errorf("name is required")
		}
		queryType := strings.ToUpper(config.Type)
		if queryType == "" {
			queryType = "A"
		}
		if !supportedDNSQueryType(queryType) {
			return fmt.Errorf("unsupported DNS query type %q", queryType)
		}
		if config.Server != "" {
			if _, _, err := net.SplitHostPort(config.Server); err != nil {
				return fmt.Errorf("DNS server must contain host and port: %w", err)
			}
		}
		return nil
	case domain.SourceICMP:
		var config ICMPConfig
		if err := decodeConfig(raw, &config); err != nil {
			return err
		}
		if strings.TrimSpace(config.Host) == "" {
			return fmt.Errorf("host is required")
		}
		switch strings.ToLower(config.Family) {
		case "", "auto", "ipv4", "4", "ipv6", "6":
			return nil
		default:
			return fmt.Errorf("family must be auto, ipv4, or ipv6")
		}
	case domain.SourceHeartbeat:
		var config HeartbeatConfig
		if err := decodeConfig(raw, &config); err != nil {
			return err
		}
		if config.ExpectedEverySeconds < int(domain.MinimumInterval.Seconds()) || config.ExpectedEverySeconds > int(domain.MaximumInterval.Seconds()) {
			return fmt.Errorf("expected interval must be between 20 and 86400 seconds")
		}
		if config.GraceSeconds < 0 || config.GraceSeconds > int(domain.MaximumInterval.Seconds()) {
			return fmt.Errorf("grace must be between 0 and 86400 seconds")
		}
		return nil
	default:
		return fmt.Errorf("unsupported source kind %q", kind)
	}
}
