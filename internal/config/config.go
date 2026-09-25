package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/infra/auth"
)

type Config struct {
	Env                          string
	HTTPHost                     string
	HTTPPort                     int
	DatabaseURL                  string
	SMTPHost                     string
	SMTPPort                     int
	SMTPUsername                 string
	SMTPPassword                 string
	SMTPFrom                     string
	LogLevel                     string
	AllowedCORSOrigins           string
	AuthIssuer                   string
	AuthAudience                 string
	AuthSecret                   string
	ServiceAuthSecret            string
	ServiceAuthIssuer            string
	ServiceAuthAudience          string
	WorkerEnabled                bool
	WorkerInterval               time.Duration
	WorkerStaleTimeout           time.Duration
	WorkerMaxAttempts            int
	SchedulerEnabled             bool
	SchedulerInterval            time.Duration
	SchedulerBatchSize           int
	CampaignWorkerEnabled        bool
	CampaignWorkerInterval       time.Duration
	CampaignWorkerBatchSize      int
	CampaignWorkerStaleTimeout   time.Duration
	AutomationSchedulerEnabled   bool
	AutomationSchedulerInterval  time.Duration
	AutomationSchedulerBatchSize int
	GMHelperAPIBaseURL           string
}

func Load() (*Config, error) {
	loadDotEnv()

	port, err := parseIntEnv("HTTP_PORT", 8080)
	if err != nil {
		return nil, err
	}

	smtpPort, err := parseIntEnvWithAliases(587, "SMTP_PORT", "SMTP_Port", "SMTP__Port")
	if err != nil {
		return nil, err
	}

	databaseURL, err := requireEnv("DATABASE_URL")
	if err != nil {
		return nil, err
	}
	smtpHost, err := requireFirstEnv("SMTP_HOST", "SMTP__Host")
	if err != nil {
		return nil, err
	}
	smtpFrom, err := requireFirstEnv("SMTP_FROM", "SMTP__From")
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

	workerMaxAttempts, err := parseIntEnv("NOTIFY_WORKER_MAX_ATTEMPTS", 5)
	if err != nil {
		return nil, err
	}

	schedulerInterval, err := parseDurationEnv("NOTIFY_SCHEDULER_INTERVAL", 5*time.Second)
	if err != nil {
		return nil, err
	}

	schedulerBatchSize, err := parseIntEnv("NOTIFY_SCHEDULER_BATCH_SIZE", 10)
	if err != nil {
		return nil, err
	}

	campaignWorkerInterval, err := parseDurationEnv("NOTIFY_CAMPAIGN_WORKER_INTERVAL", 5*time.Second)
	if err != nil {
		return nil, err
	}

	campaignWorkerBatchSize, err := parseIntEnv("NOTIFY_CAMPAIGN_WORKER_BATCH_SIZE", 20)
	if err != nil {
		return nil, err
	}

	campaignWorkerStaleTimeout, err := parseDurationEnv("NOTIFY_CAMPAIGN_WORKER_STALE_TIMEOUT", 5*time.Minute)
	if err != nil {
		return nil, err
	}

	automationSchedulerInterval, err := parseDurationEnv("NOTIFY_AUTOMATION_SCHEDULER_INTERVAL", 1*time.Hour)
	if err != nil {
		return nil, err
	}

	automationSchedulerBatchSize, err := parseIntEnv("NOTIFY_AUTOMATION_SCHEDULER_BATCH_SIZE", 250)
	if err != nil {
		return nil, err
	}

	authSecret := envOrDefault("NOTIFY_AUTH_SECRET", "Z21oZWxwZXItZGVmYXVsdC1qd3Qtc2VjcmV0LTMyYiE=")
	serviceAuthSecret := envOrDefault("NOTIFY_SERVICE_AUTH_SECRET", authSecret)

	cfg := &Config{
		Env:                          envOrDefault("APP_ENV", "development"),
		HTTPHost:                     envOrDefault("HTTP_HOST", "0.0.0.0"),
		HTTPPort:                     port,
		DatabaseURL:                  databaseURL,
		SMTPHost:                     smtpHost,
		SMTPPort:                     smtpPort,
		SMTPUsername:                 getFirstEnv("SMTP_USERNAME", "SMTP__Username"),
		SMTPPassword:                 getFirstEnv("SMTP_PASSWORD", "SMTP__Password"),
		SMTPFrom:                     smtpFrom,
		LogLevel:                     envOrDefault("LOG_LEVEL", "info"),
		AllowedCORSOrigins:           envOrDefault("ALLOWED_CORS_ORIGINS", "*"),
		AuthIssuer:                   envOrDefault("NOTIFY_AUTH_ISSUER", "gmhelper-api"),
		AuthAudience:                 envOrDefault("NOTIFY_AUTH_AUDIENCE", "gmhelper-notify-api"),
		AuthSecret:                   authSecret,
		ServiceAuthSecret:            serviceAuthSecret,
		ServiceAuthIssuer:            envOrDefault("NOTIFY_SERVICE_AUTH_ISSUER", envOrDefault("NOTIFY_AUTH_ISSUER", "gmhelper-api")),
		ServiceAuthAudience:          envOrDefault("NOTIFY_SERVICE_AUTH_AUDIENCE", envOrDefault("NOTIFY_AUTH_AUDIENCE", "gmhelper-notify-api")),
		WorkerEnabled:                parseBoolEnv("NOTIFY_WORKER_ENABLED", true),
		WorkerInterval:               workerInterval,
		WorkerStaleTimeout:           workerStaleTimeout,
		WorkerMaxAttempts:            workerMaxAttempts,
		SchedulerEnabled:             parseBoolEnv("NOTIFY_SCHEDULER_ENABLED", true),
		SchedulerInterval:            schedulerInterval,
		SchedulerBatchSize:           schedulerBatchSize,
		CampaignWorkerEnabled:        parseBoolEnv("NOTIFY_CAMPAIGN_WORKER_ENABLED", true),
		CampaignWorkerInterval:       campaignWorkerInterval,
		CampaignWorkerBatchSize:      campaignWorkerBatchSize,
		CampaignWorkerStaleTimeout:   campaignWorkerStaleTimeout,
		AutomationSchedulerEnabled:   parseBoolEnv("NOTIFY_AUTOMATION_SCHEDULER_ENABLED", true),
		AutomationSchedulerInterval:  automationSchedulerInterval,
		AutomationSchedulerBatchSize: automationSchedulerBatchSize,
		GMHelperAPIBaseURL:           envOrDefault("GMHELPER_API_BASE_URL", envOrDefault("NOTIFY_GMHELPER_API_BASE_URL", "")),
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

var envKeyAliases = map[string][]string{
	"SMTP_HOST":      {"SMTP__Host"},
	"SMTP__Host":     {"SMTP_HOST"},
	"SMTP_PORT":      {"SMTP_Port", "SMTP__Port"},
	"SMTP_Port":      {"SMTP_PORT", "SMTP__Port"},
	"SMTP_USERNAME":  {"SMTP__Username"},
	"SMTP__Username": {"SMTP_USERNAME"},
	"SMTP_PASSWORD":  {"SMTP__Password"},
	"SMTP__Password": {"SMTP_PASSWORD"},
	"SMTP_FROM":      {"SMTP__From"},
	"SMTP__From":     {"SMTP_FROM"},
}

func hasEnvOrAlias(key string) bool {
	if _, exists := os.LookupEnv(key); exists {
		return true
	}
	if aliases, ok := envKeyAliases[key]; ok {
		for _, a := range aliases {
			if _, exists := os.LookupEnv(a); exists {
				return true
			}
		}
	}
	return false
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

		if !hasEnvOrAlias(key) {
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
	if strings.TrimSpace(c.AuthSecret) == "" {
		return fmt.Errorf("NOTIFY_AUTH_SECRET cannot be empty")
	}
	if _, err := auth.DecodeSecretKey(c.AuthSecret); err != nil {
		return fmt.Errorf("NOTIFY_AUTH_SECRET is invalid: %w", err)
	}
	if strings.TrimSpace(c.ServiceAuthSecret) == "" {
		c.ServiceAuthSecret = c.AuthSecret
	}
	if strings.TrimSpace(c.ServiceAuthIssuer) == "" {
		if strings.TrimSpace(c.AuthIssuer) != "" {
			c.ServiceAuthIssuer = c.AuthIssuer
		} else {
			c.ServiceAuthIssuer = "gmhelper-api"
		}
	}
	if strings.TrimSpace(c.ServiceAuthAudience) == "" {
		if strings.TrimSpace(c.AuthAudience) != "" {
			c.ServiceAuthAudience = c.AuthAudience
		} else {
			c.ServiceAuthAudience = "gmhelper-notify-api"
		}
	}
	if c.Env == "production" {
		if strings.TrimSpace(c.AuthSecret) == "" || c.AuthSecret == "Z21oZWxwZXItZGVmYXVsdC1qd3Qtc2VjcmV0LTMyYiE=" {
			return fmt.Errorf("NOTIFY_AUTH_SECRET must be explicitly configured in production environment")
		}
		if strings.TrimSpace(c.ServiceAuthSecret) == "" || c.ServiceAuthSecret == "Z21oZWxwZXItZGVmYXVsdC1qd3Qtc2VjcmV0LTMyYiE=" {
			return fmt.Errorf("NOTIFY_SERVICE_AUTH_SECRET must be explicitly configured in production environment")
		}
	}
	if _, err := auth.DecodeSecretKey(c.ServiceAuthSecret); err != nil {
		return fmt.Errorf("NOTIFY_SERVICE_AUTH_SECRET is invalid: %w", err)
	}
	if strings.TrimSpace(c.ServiceAuthIssuer) == "" {
		return fmt.Errorf("NOTIFY_SERVICE_AUTH_ISSUER cannot be empty")
	}
	if strings.TrimSpace(c.ServiceAuthAudience) == "" {
		return fmt.Errorf("NOTIFY_SERVICE_AUTH_AUDIENCE cannot be empty")
	}
	if c.WorkerEnabled && c.WorkerInterval <= 0 {
		return fmt.Errorf("NOTIFY_WORKER_INTERVAL must be a positive duration when worker is enabled")
	}
	if c.WorkerEnabled && c.WorkerStaleTimeout <= 0 {
		return fmt.Errorf("NOTIFY_WORKER_STALE_TIMEOUT must be a positive duration when worker is enabled")
	}
	if c.WorkerEnabled && c.WorkerMaxAttempts <= 0 {
		return fmt.Errorf("NOTIFY_WORKER_MAX_ATTEMPTS must be a positive integer when worker is enabled")
	}
	if c.SchedulerEnabled && c.SchedulerInterval <= 0 {
		return fmt.Errorf("NOTIFY_SCHEDULER_INTERVAL must be a positive duration when scheduler is enabled")
	}
	if c.SchedulerEnabled && c.SchedulerBatchSize <= 0 {
		return fmt.Errorf("NOTIFY_SCHEDULER_BATCH_SIZE must be a positive integer when scheduler is enabled")
	}
	if c.CampaignWorkerEnabled && c.CampaignWorkerInterval <= 0 {
		return fmt.Errorf("NOTIFY_CAMPAIGN_WORKER_INTERVAL must be a positive duration when campaign worker is enabled")
	}
	if c.CampaignWorkerEnabled && c.CampaignWorkerBatchSize <= 0 {
		return fmt.Errorf("NOTIFY_CAMPAIGN_WORKER_BATCH_SIZE must be a positive integer when campaign worker is enabled")
	}
	if c.CampaignWorkerEnabled && c.CampaignWorkerStaleTimeout <= 0 {
		return fmt.Errorf("NOTIFY_CAMPAIGN_WORKER_STALE_TIMEOUT must be a positive duration when campaign worker is enabled")
	}
	if c.AutomationSchedulerEnabled && c.AutomationSchedulerInterval <= 0 {
		return fmt.Errorf("NOTIFY_AUTOMATION_SCHEDULER_INTERVAL must be a positive duration when automation scheduler is enabled")
	}
	if c.AutomationSchedulerEnabled && c.AutomationSchedulerBatchSize <= 0 {
		return fmt.Errorf("NOTIFY_AUTOMATION_SCHEDULER_BATCH_SIZE must be a positive integer when automation scheduler is enabled")
	}
	if strings.TrimSpace(c.GMHelperAPIBaseURL) != "" {
		parsed, err := url.ParseRequestURI(strings.TrimSpace(c.GMHelperAPIBaseURL))
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return fmt.Errorf("invalid GMHELPER_API_BASE_URL: must be a valid http or https URL with host")
		}
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

func getFirstEnv(keys ...string) string {
	for _, k := range keys {
		if val := os.Getenv(k); strings.TrimSpace(val) != "" {
			return val
		}
	}
	return ""
}

func requireFirstEnv(primary string, aliases ...string) (string, error) {
	if val := os.Getenv(primary); strings.TrimSpace(val) != "" {
		return val, nil
	}
	for _, a := range aliases {
		if val := os.Getenv(a); strings.TrimSpace(val) != "" {
			return val, nil
		}
	}
	return "", fmt.Errorf("missing required environment variable: %s", primary)
}

func parseIntEnvWithAliases(defaultValue int, primary string, aliases ...string) (int, error) {
	val := getFirstEnv(append([]string{primary}, aliases...)...)
	if val == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("invalid integer for %s: %w", primary, err)
	}
	return parsed, nil
}
