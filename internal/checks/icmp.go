package checks

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"os"
	"strings"
	"time"

	"github.com/M0okz/cairnops/internal/domain"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

type ICMP struct{}

type ICMPConfig struct {
	Host   string `json:"host"`
	Family string `json:"family,omitempty"`
}

func (ICMP) Check(ctx context.Context, source domain.Source) domain.Observation {
	startedAt := time.Now()
	var config ICMPConfig
	if err := decodeConfig(source.Config, &config); err != nil {
		return unknown(source, startedAt, "invalid_config", err)
	}
	if strings.TrimSpace(config.Host) == "" {
		return unknown(source, startedAt, "invalid_config", fmt.Errorf("host is required"))
	}

	ip, network, protocol, requestType, replyType, err := resolveICMPTarget(ctx, config.Host, config.Family)
	if err != nil {
		return unhealthy(source, startedAt, "icmp_resolve_failed", err)
	}
	connection, err := icmp.ListenPacket(network, wildcardAddress(network))
	if err != nil {
		if errors.Is(err, os.ErrPermission) || strings.Contains(strings.ToLower(err.Error()), "operation not permitted") {
			return unknown(source, startedAt, "icmp_unavailable", fmt.Errorf("ICMP socket is not permitted: %w", err))
		}
		return unknown(source, startedAt, "icmp_unavailable", err)
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(source.Timeout))

	sequence := rand.IntN(65535)
	message := icmp.Message{
		Type: requestType,
		Code: 0,
		Body: &icmp.Echo{ID: os.Getpid() & 0xffff, Seq: sequence, Data: []byte("cairnops")},
	}
	payload, err := message.Marshal(nil)
	if err != nil {
		return unknown(source, startedAt, "icmp_encode_failed", err)
	}
	if _, err := connection.WriteTo(payload, &net.UDPAddr{IP: ip}); err != nil {
		return unhealthy(source, startedAt, "icmp_send_failed", err)
	}

	buffer := make([]byte, 1500)
	for {
		count, peer, readErr := connection.ReadFrom(buffer)
		if readErr != nil {
			return unhealthy(source, startedAt, "icmp_timeout", readErr)
		}
		reply, parseErr := icmp.ParseMessage(protocol, buffer[:count])
		if parseErr != nil || reply.Type != replyType {
			continue
		}
		echo, ok := reply.Body.(*icmp.Echo)
		if !ok || echo.Seq != sequence {
			continue
		}
		observation := healthy(source, startedAt)
		observation.Details["address"] = ip.String()
		observation.Details["peer"] = peer.String()
		observation.Details["family"] = strings.TrimPrefix(network, "udp")
		return observation
	}
}

func resolveICMPTarget(ctx context.Context, host, family string) (net.IP, string, int, icmp.Type, icmp.Type, error) {
	network := "ip"
	switch strings.ToLower(family) {
	case "", "auto":
	case "ipv4", "4":
		network = "ip4"
	case "ipv6", "6":
		network = "ip6"
	default:
		return nil, "", 0, nil, nil, fmt.Errorf("family must be auto, ipv4, or ipv6")
	}
	addresses, err := net.DefaultResolver.LookupIP(ctx, network, host)
	if err != nil {
		return nil, "", 0, nil, nil, err
	}
	if len(addresses) == 0 {
		return nil, "", 0, nil, nil, fmt.Errorf("host resolved without an address")
	}
	ip := addresses[0]
	if ip.To4() != nil {
		return ip, "udp4", 1, ipv4.ICMPTypeEcho, ipv4.ICMPTypeEchoReply, nil
	}
	return ip, "udp6", 58, ipv6.ICMPTypeEchoRequest, ipv6.ICMPTypeEchoReply, nil
}

func wildcardAddress(network string) string {
	if network == "udp6" {
		return "::"
	}
	return "0.0.0.0"
}
