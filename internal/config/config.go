package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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

func requireEnv(name string) (string, error) {
	value := os.Getenv(name)
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("missing required environment variable: %s", name)
	}
	return value, nil
}

