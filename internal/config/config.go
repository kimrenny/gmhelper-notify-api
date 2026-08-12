package config

import (
	"fmt"
	"os"
	"strconv"
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

	return &Config{
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
	}, nil
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
	if value == "" {
		return "", fmt.Errorf("missing required environment variable: %s", name)
	}
	return value, nil
}
