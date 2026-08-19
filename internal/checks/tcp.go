package checks

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/M0okz/cairnops/internal/domain"
)

const maximumTCPResponse = 64 * 1024

type TCP struct{}

type TCPConfig struct {
	Address        string `json:"address"`
	TLS            bool   `json:"tls,omitempty"`
	ServerName     string `json:"server_name,omitempty"`
	Send           string `json:"send,omitempty"`
	ExpectContains string `json:"expect_contains,omitempty"`
}

func (TCP) Check(ctx context.Context, source domain.Source) domain.Observation {
	startedAt := time.Now()
	var config TCPConfig
	if err := decodeConfig(source.Config, &config); err != nil {
		return unknown(source, startedAt, "invalid_config", err)
	}
	if _, _, err := net.SplitHostPort(config.Address); err != nil {
		return unknown(source, startedAt, "invalid_config", fmt.Errorf("address must contain host and port: %w", err))
	}

	dialer := &net.Dialer{Timeout: source.Timeout}
	var connection net.Conn
	var err error
	if config.TLS {
		tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: config.ServerName}
		connection, err = (&tls.Dialer{NetDialer: dialer, Config: tlsConfig}).DialContext(ctx, "tcp", config.Address)
	} else {
		connection, err = dialer.DialContext(ctx, "tcp", config.Address)
	}
	if err != nil {
		return unhealthy(source, startedAt, "tcp_connect_failed", err)
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(source.Timeout))

	if config.Send != "" {
		if _, err := io.WriteString(connection, config.Send); err != nil {
			return unhealthy(source, startedAt, "tcp_write_failed", err)
		}
	}
	if config.ExpectContains == "" {
		return healthy(source, startedAt)
	}

	response := make([]byte, 0, 4096)
	buffer := make([]byte, 4096)
	for {
		count, readErr := connection.Read(buffer)
		response = append(response, buffer[:count]...)
		if len(response) > maximumTCPResponse {
			return unhealthy(source, startedAt, "response_too_large", fmt.Errorf("response exceeds %d bytes", maximumTCPResponse))
		}
		if bytes.Contains(response, []byte(config.ExpectContains)) {
			observation := healthy(source, startedAt)
			observation.Details["response_bytes"] = len(response)
			return observation
		}
		if readErr != nil {
			if readErr != io.EOF {
				if networkError, ok := readErr.(net.Error); !ok || !networkError.Timeout() || len(response) == 0 {
					return unhealthy(source, startedAt, "tcp_read_failed", readErr)
				}
			}
			observation := unhealthy(source, startedAt, "content_missing", fmt.Errorf("expected TCP response content was not found"))
			observation.Details["response_bytes"] = len(response)
			return observation
		}
	}
}
