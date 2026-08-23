package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env                string
	HTTPHost           string
	HTTPPort           int
	DatabaseURL        string
	SMTPHost           string
	SMTPPort           int
	SMTPUsername       string
	SMTPPassword       string
	SMTPFrom           string
	LogLevel           string
	AllowedCORSOrigins string
	AuthIssuer         string
	AuthAudience       string
	AuthSecret         string
	WorkerEnabled      bool
	WorkerInterval     time.Duration
	WorkerStaleTimeout time.Duration
}

func Load() (*Config, error) {
	loadDotEnv()

	port, err := parseIntEnv("HTTP_PORT", 8080)
	if err != nil {
		return nil, err
	}

	smtpPort, err := parseIntEnv("SMTP_PORT", 587)
	if err != nil {
		return nil, err
	}

	databaseURL, err := requireEnv("DATABASE_URL")
	if err != nil {
		return nil, err
	}
	smtpHost, err := requireEnv("SMTP_HOST")
	if err != nil {
		return nil, err
	}
	smtpFrom, err := requireEnv("SMTP_FROM")
	if err != nil {
		return nil, err
	}

	workerInterval, err := parseDurationEnv("NOTIFY_WORKER_INTERVAL", 5*time.Second)
	if err != nil {
		return nil, err
	}

	workerStaleTimeout, err := parseDurationEnv("NOTIFY_WORKER_STALE_TIMEOUT", 5*time.Minute)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Env:                envOrDefault("APP_ENV", "development"),
		HTTPHost:           envOrDefault("HTTP_HOST", "0.0.0.0"),
		HTTPPort:           port,
		DatabaseURL:        databaseURL,
		SMTPHost:           smtpHost,
		SMTPPort:           smtpPort,
		SMTPUsername:       os.Getenv("SMTP_USERNAME"),
		SMTPPassword:       os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:           smtpFrom,
		LogLevel:           envOrDefault("LOG_LEVEL", "info"),
		AllowedCORSOrigins: envOrDefault("ALLOWED_CORS_ORIGINS", "*"),
		AuthIssuer:         envOrDefault("NOTIFY_AUTH_ISSUER", "gmhelper-api"),
		AuthAudience:       envOrDefault("NOTIFY_AUTH_AUDIENCE", "gmhelper-notify-api"),
		AuthSecret:         envOrDefault("NOTIFY_AUTH_SECRET", "gmhelper-secret-key-change-in-production"),
		WorkerEnabled:      parseBoolEnv("NOTIFY_WORKER_ENABLED", true),
		WorkerInterval:     workerInterval,
		WorkerStaleTimeout: workerStaleTimeout,
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

func loadDotEnv() {
	candidates := []string{".env", "../.env", "../../.env"}
	for _, path := range candidates {
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			_ = parseAndApplyEnvFile(path)
			return
		}
	}
}

func parseAndApplyEnvFile(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}

		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
	return nil
}

func (c *Config) Validate() error {
	if err := validatePort("HTTP_PORT", c.HTTPPort); err != nil {
		return err
	}
	if err := validatePort("SMTP_PORT", c.SMTPPort); err != nil {
		return err
	}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return fmt.Errorf("DATABASE_URL cannot be empty")
	}
	if strings.TrimSpace(c.SMTPHost) == "" {
		return fmt.Errorf("SMTP_HOST cannot be empty")
	}
	if strings.TrimSpace(c.SMTPFrom) == "" {
		return fmt.Errorf("SMTP_FROM cannot be empty")
	}
	if c.Env == "production" {
		if strings.TrimSpace(c.AuthSecret) == "" || c.AuthSecret == "gmhelper-secret-key-change-in-production" {
			return fmt.Errorf("NOTIFY_AUTH_SECRET must be explicitly configured in production environment")
		}
	}
	if strings.TrimSpace(c.AuthSecret) == "" {
		return fmt.Errorf("NOTIFY_AUTH_SECRET cannot be empty")
	}
	if c.WorkerEnabled && c.WorkerInterval <= 0 {
		return fmt.Errorf("NOTIFY_WORKER_INTERVAL must be a positive duration when worker is enabled")
	}
	if c.WorkerEnabled && c.WorkerStaleTimeout <= 0 {
		return fmt.Errorf("NOTIFY_WORKER_STALE_TIMEOUT must be a positive duration when worker is enabled")
	}
	return nil
}

func validatePort(name string, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid port for %s: %d (must be between 1 and 65535)", name, port)
	}
	return nil
}

func envOrDefault(name, defaultValue string) string {
	value := os.Getenv(name)
	if value == "" {
		return defaultValue
	}
	return value
}

func parseIntEnv(name string, defaultValue int) (int, error) {
	value := os.Getenv(name)
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid integer for %s: %w", name, err)
	}
	return parsed, nil
}

func parseBoolEnv(name string, defaultValue bool) bool {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func parseDurationEnv(name string, defaultValue time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue, nil
	}
	d, err := time.ParseDuration(value)
	if err == nil {
		return d, nil
	}
	sec, intErr := strconv.Atoi(value)
	if intErr == nil {
		return time.Duration(sec) * time.Second, nil
	}
	return 0, fmt.Errorf("invalid duration for %s: %w", name, err)
}

func requireEnv(name string) (string, error) {
	value := os.Getenv(name)
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("missing required environment variable: %s", name)
	}
	return value, nil
}
