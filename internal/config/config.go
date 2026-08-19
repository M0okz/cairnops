package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

const defaultDatabaseURL = "postgres://cairnops:cairnops@127.0.0.1:5432/cairnops?sslmode=disable"

type Server struct {
	HTTPAddress    string
	DatabaseURL    string
	WebDir         string
	PublicURL      string
	BootstrapToken string
	MasterKeyFile  string
}

type Worker struct {
	HealthAddress string
	DatabaseURL   string
	InstanceID    string
	PublicURL     string
	MasterKeyFile string
}

func LoadServer() (Server, error) {
	cfg := Server{
		HTTPAddress:    envOr("CAIRNOPS_HTTP_ADDRESS", ":8080"),
		DatabaseURL:    envOr("CAIRNOPS_DATABASE_URL", defaultDatabaseURL),
		WebDir:         envOr("CAIRNOPS_WEB_DIR", "web/build"),
		PublicURL:      envOr("CAIRNOPS_PUBLIC_URL", "http://localhost:8080"),
		BootstrapToken: envOr("CAIRNOPS_BOOTSTRAP_TOKEN", ""),
		MasterKeyFile:  envOr("CAIRNOPS_MASTER_KEY_FILE", "/var/lib/cairnops/master.key"),
	}
	if err := validateAddress(cfg.HTTPAddress); err != nil {
		return Server{}, fmt.Errorf("CAIRNOPS_HTTP_ADDRESS: %w", err)
	}
	if len(cfg.BootstrapToken) < 32 {
		return Server{}, fmt.Errorf("CAIRNOPS_BOOTSTRAP_TOKEN must contain at least 32 characters")
	}
	if cfg.MasterKeyFile == "" {
		return Server{}, fmt.Errorf("CAIRNOPS_MASTER_KEY_FILE must not be empty")
	}
	if err := validatePublicURL(cfg.PublicURL); err != nil {
		return Server{}, fmt.Errorf("CAIRNOPS_PUBLIC_URL: %w", err)
	}
	return cfg, nil
}

func LoadWorker() (Worker, error) {
	instanceID, err := workerInstanceID()
	if err != nil {
		return Worker{}, err
	}
	cfg := Worker{
		HealthAddress: envOr("CAIRNOPS_WORKER_HEALTH_ADDRESS", ":8081"),
		DatabaseURL:   envOr("CAIRNOPS_DATABASE_URL", defaultDatabaseURL),
		InstanceID:    instanceID,
		PublicURL:     envOr("CAIRNOPS_PUBLIC_URL", "http://localhost:8080"),
		MasterKeyFile: envOr("CAIRNOPS_MASTER_KEY_FILE", "/var/lib/cairnops/master.key"),
	}
	if err := validateAddress(cfg.HealthAddress); err != nil {
		return Worker{}, fmt.Errorf("CAIRNOPS_WORKER_HEALTH_ADDRESS: %w", err)
	}
	if strings.TrimSpace(cfg.InstanceID) == "" {
		return Worker{}, fmt.Errorf("CAIRNOPS_INSTANCE_ID must not be empty")
	}
	if err := validatePublicURL(cfg.PublicURL); err != nil {
		return Worker{}, fmt.Errorf("CAIRNOPS_PUBLIC_URL: %w", err)
	}
	if cfg.MasterKeyFile == "" {
		return Worker{}, fmt.Errorf("CAIRNOPS_MASTER_KEY_FILE must not be empty")
	}
	return cfg, nil
}

func workerInstanceID() (string, error) {
	if value, ok := os.LookupEnv("CAIRNOPS_INSTANCE_ID"); ok {
		return strings.TrimSpace(value), nil
	}
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("derive worker instance ID from hostname: %w", err)
	}
	return strings.TrimSpace(hostname), nil
}

func envOr(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return strings.TrimSpace(value)
	}
	return fallback
}

func validateAddress(address string) error {
	if strings.TrimSpace(address) == "" {
		return fmt.Errorf("must not be empty")
	}
	if !strings.Contains(address, ":") {
		return fmt.Errorf("must contain a port")
	}
	return nil
}

func validatePublicURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("must be an absolute HTTP or HTTPS URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("scheme must be http or https")
	}
	if parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("must contain only a scheme and host")
	}
	if parsed.Scheme == "http" {
		host := strings.ToLower(parsed.Hostname())
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return fmt.Errorf("http is allowed only for localhost; use https for remote access")
		}
	}
	return nil
}
