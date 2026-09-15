package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfigLoad_Success(t *testing.T) {
	oldDB := os.Getenv("DATABASE_URL")
	oldHost := os.Getenv("SMTP_HOST")
	oldPort := os.Getenv("SMTP_PORT")
	oldFrom := os.Getenv("SMTP_FROM")
	oldHTTPPort := os.Getenv("HTTP_PORT")

	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Setenv("SMTP_HOST", "smtp.example.com")
	os.Setenv("SMTP_FROM", "test@example.com")
	os.Setenv("HTTP_PORT", "9090")
	os.Setenv("SMTP_PORT", "2525")
	defer func() {
		restoreEnv("DATABASE_URL", oldDB)
		restoreEnv("SMTP_HOST", oldHost)
		restoreEnv("SMTP_PORT", oldPort)
		restoreEnv("SMTP_FROM", oldFrom)
		restoreEnv("HTTP_PORT", oldHTTPPort)
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected config load success, got: %v", err)
	}

	if cfg.HTTPPort != 9090 {
		t.Errorf("expected HTTPPort 9090, got %d", cfg.HTTPPort)
	}
	if cfg.SMTPPort != 2525 {
		t.Errorf("expected SMTPPort 2525, got %d", cfg.SMTPPort)
	}
	if cfg.DatabaseURL != "postgres://localhost/test" {
		t.Errorf("expected DatabaseURL 'postgres://localhost/test', got %s", cfg.DatabaseURL)
	}
}

func TestConfigLoad_SMTPDotNetAliases(t *testing.T) {
	oldDB := os.Getenv("DATABASE_URL")
	oldHost := os.Getenv("SMTP_HOST")
	oldPort := os.Getenv("SMTP_PORT")
	oldFrom := os.Getenv("SMTP_FROM")
	oldUser := os.Getenv("SMTP_USERNAME")
	oldPass := os.Getenv("SMTP_PASSWORD")

	os.Unsetenv("SMTP_HOST")
	os.Unsetenv("SMTP_PORT")
	os.Unsetenv("SMTP_FROM")
	os.Unsetenv("SMTP_USERNAME")
	os.Unsetenv("SMTP_PASSWORD")

	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Setenv("SMTP__Host", "smtp.gmail.com")
	os.Setenv("SMTP_Port", "587")
	os.Setenv("SMTP__Username", "noreply.gmhelper@gmail.com")
	os.Setenv("SMTP__Password", "secret-pass")
	os.Setenv("SMTP__From", "noreply.gmhelper@gmail.com")
	defer func() {
		os.Unsetenv("SMTP__Host")
		os.Unsetenv("SMTP_Port")
		os.Unsetenv("SMTP__Username")
		os.Unsetenv("SMTP__Password")
		os.Unsetenv("SMTP__From")
		restoreEnv("DATABASE_URL", oldDB)
		restoreEnv("SMTP_HOST", oldHost)
		restoreEnv("SMTP_PORT", oldPort)
		restoreEnv("SMTP_FROM", oldFrom)
		restoreEnv("SMTP_USERNAME", oldUser)
		restoreEnv("SMTP_PASSWORD", oldPass)
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected config load success with dot net aliases, got: %v", err)
	}

	if cfg.SMTPHost != "smtp.gmail.com" {
		t.Errorf("expected SMTPHost 'smtp.gmail.com', got '%s'", cfg.SMTPHost)
	}
	if cfg.SMTPPort != 587 {
		t.Errorf("expected SMTPPort 587, got %d", cfg.SMTPPort)
	}
	if cfg.SMTPUsername != "noreply.gmhelper@gmail.com" {
		t.Errorf("expected SMTPUsername 'noreply.gmhelper@gmail.com', got '%s'", cfg.SMTPUsername)
	}
	if cfg.SMTPPassword != "secret-pass" {
		t.Errorf("expected SMTPPassword 'secret-pass', got '%s'", cfg.SMTPPassword)
	}
	if cfg.SMTPFrom != "noreply.gmhelper@gmail.com" {
		t.Errorf("expected SMTPFrom 'noreply.gmhelper@gmail.com', got '%s'", cfg.SMTPFrom)
	}
}

func restoreEnv(key, val string) {
	if val == "" {
		os.Unsetenv(key)
	} else {
		os.Setenv(key, val)
	}
}

func TestParseAndApplyEnvFile(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env.test")
	content := []byte(`
# Test comment
TEST_CUSTOM_VAR=hello_world
TEST_QUOTED_VAR="quoted value"
`)
	if err := os.WriteFile(envPath, content, 0644); err != nil {
		t.Fatalf("failed to write test env file: %v", err)
	}
	defer func() {
		os.Unsetenv("TEST_CUSTOM_VAR")
		os.Unsetenv("TEST_QUOTED_VAR")
	}()

	if err := parseAndApplyEnvFile(envPath); err != nil {
		t.Fatalf("parseAndApplyEnvFile failed: %v", err)
	}

	if os.Getenv("TEST_CUSTOM_VAR") != "hello_world" {
		t.Errorf("expected TEST_CUSTOM_VAR 'hello_world', got %s", os.Getenv("TEST_CUSTOM_VAR"))
	}
	if os.Getenv("TEST_QUOTED_VAR") != "quoted value" {
		t.Errorf("expected TEST_QUOTED_VAR 'quoted value', got %s", os.Getenv("TEST_QUOTED_VAR"))
	}
}

func TestConfigValidate_MissingRequired(t *testing.T) {
	cfg := &Config{
		HTTPPort: 8080,
		SMTPPort: 587,
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error due to missing required variables, got nil")
	}
}

func TestConfigValidate_InvalidPort(t *testing.T) {
	cfg := &Config{
		DatabaseURL: "postgres://localhost/test",
		SMTPHost:    "smtp.example.com",
		SMTPFrom:    "test@example.com",
		HTTPPort:    70000,
		SMTPPort:    587,
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for port > 65535, got nil")
	}
}

func TestConfigValidate_ProductionAuthSecretRequired(t *testing.T) {
	// Default dev secret rejected in production
	cfgDefaultSecret := &Config{
		Env:         "production",
		DatabaseURL: "postgres://localhost/test",
		SMTPHost:    "smtp.example.com",
		SMTPFrom:    "test@example.com",
		HTTPPort:    8080,
		SMTPPort:    587,
		AuthSecret:  "Z21oZWxwZXItZGVmYXVsdC1qd3Qtc2VjcmV0LTMyYiE=",
	}
	if err := cfgDefaultSecret.Validate(); err == nil {
		t.Fatal("expected error in production when using default secret, got nil")
	}

	// Empty secret rejected in production
	cfgEmptySecret := &Config{
		Env:         "production",
		DatabaseURL: "postgres://localhost/test",
		SMTPHost:    "smtp.example.com",
		SMTPFrom:    "test@example.com",
		HTTPPort:    8080,
		SMTPPort:    587,
		AuthSecret:  "",
	}
	if err := cfgEmptySecret.Validate(); err == nil {
		t.Fatal("expected error in production when auth secret is empty, got nil")
	}

	// Invalid base64 secret rejected
	cfgInvalidBase64 := &Config{
		Env:         "production",
		DatabaseURL: "postgres://localhost/test",
		SMTPHost:    "smtp.example.com",
		SMTPFrom:    "test@example.com",
		HTTPPort:    8080,
		SMTPPort:    587,
		AuthSecret:  "not-valid-base64-secret!!",
	}
	if err := cfgInvalidBase64.Validate(); err == nil {
		t.Fatal("expected error when auth secret is invalid base64, got nil")
	}

	// Explicit production secret allowed
	cfgValidProd := &Config{
		Env:                 "production",
		DatabaseURL:         "postgres://localhost/test",
		SMTPHost:            "smtp.example.com",
		SMTPFrom:            "test@example.com",
		HTTPPort:            8080,
		SMTPPort:            587,
		AuthSecret:          "c3VwZXItc2VjdXJlLXByb2R1Y3Rpb24tc2VjcmV0LXZhbHVl",
		ServiceAuthSecret:   "c3VwZXItc2VjdXJlLXByb2R1Y3Rpb24tc2VjcmV0LXZhbHVl",
		ServiceAuthAudience: "gmhelper-api",
	}
	if err := cfgValidProd.Validate(); err != nil {
		t.Fatalf("expected valid production config, got: %v", err)
	}

	// Default dev service secret rejected in production
	cfgDefaultServiceSecret := &Config{
		Env:                 "production",
		DatabaseURL:         "postgres://localhost/test",
		SMTPHost:            "smtp.example.com",
		SMTPFrom:            "test@example.com",
		HTTPPort:            8080,
		SMTPPort:            587,
		AuthSecret:          "c3VwZXItc2VjdXJlLXByb2R1Y3Rpb24tc2VjcmV0LXZhbHVl",
		ServiceAuthSecret:   "Z21oZWxwZXItZGVmYXVsdC1qd3Qtc2VjcmV0LTMyYiE=",
		ServiceAuthAudience: "gmhelper-api",
	}
	if err := cfgDefaultServiceSecret.Validate(); err == nil {
		t.Fatal("expected error in production when using default service auth secret, got nil")
	}
}

func TestConfigValidate_WorkerSettings(t *testing.T) {
	validSecret := "dGVzdC1zZWNyZXQta2V5LTMyLWJ5dGVzLWxvbmchIQ=="

	// Worker enabled with zero/negative interval -> error
	cfgInvalidInterval := &Config{
		DatabaseURL:    "postgres://localhost/test",
		SMTPHost:       "smtp.example.com",
		SMTPFrom:       "test@example.com",
		HTTPPort:       8080,
		SMTPPort:       587,
		AuthSecret:     validSecret,
		WorkerEnabled:  true,
		WorkerInterval: 0,
	}
	if err := cfgInvalidInterval.Validate(); err == nil {
		t.Fatal("expected error when worker is enabled with zero interval, got nil")
	}

	// Worker disabled with zero interval -> ok
	cfgDisabled := &Config{
		DatabaseURL:    "postgres://localhost/test",
		SMTPHost:       "smtp.example.com",
		SMTPFrom:       "test@example.com",
		HTTPPort:       8080,
		SMTPPort:       587,
		AuthSecret:     validSecret,
		WorkerEnabled:  false,
		WorkerInterval: 0,
	}
	if err := cfgDisabled.Validate(); err != nil {
		t.Fatalf("expected valid config when worker is disabled, got: %v", err)
	}

	// Worker enabled with valid interval but zero stale timeout -> error
	cfgInvalidStaleTimeout := &Config{
		DatabaseURL:        "postgres://localhost/test",
		SMTPHost:           "smtp.example.com",
		SMTPFrom:           "test@example.com",
		HTTPPort:           8080,
		SMTPPort:           587,
		AuthSecret:         validSecret,
		WorkerEnabled:      true,
		WorkerInterval:     5 * time.Second,
		WorkerStaleTimeout: 0,
	}
	if err := cfgInvalidStaleTimeout.Validate(); err == nil {
		t.Fatal("expected error when worker is enabled with zero stale timeout, got nil")
	}

	// Worker enabled with zero max attempts -> error
	cfgInvalidMaxAttempts := &Config{
		DatabaseURL:        "postgres://localhost/test",
		SMTPHost:           "smtp.example.com",
		SMTPFrom:           "test@example.com",
		HTTPPort:           8080,
		SMTPPort:           587,
		AuthSecret:         validSecret,
		WorkerEnabled:      true,
		WorkerInterval:     5 * time.Second,
		WorkerStaleTimeout: 5 * time.Minute,
		WorkerMaxAttempts:  0,
	}
	if err := cfgInvalidMaxAttempts.Validate(); err == nil {
		t.Fatal("expected error when worker is enabled with zero max attempts, got nil")
	}

	// Worker enabled with valid settings -> ok
	cfgValidWorker := &Config{
		DatabaseURL:        "postgres://localhost/test",
		SMTPHost:           "smtp.example.com",
		SMTPFrom:           "test@example.com",
		HTTPPort:           8080,
		SMTPPort:           587,
		AuthSecret:         validSecret,
		WorkerEnabled:      true,
		WorkerInterval:     5 * time.Second,
		WorkerStaleTimeout: 5 * time.Minute,
		WorkerMaxAttempts:  5,
	}
	if err := cfgValidWorker.Validate(); err != nil {
		t.Fatalf("expected valid config for enabled worker with positive interval, stale timeout, and max attempts, got: %v", err)
	}
}

func TestConfigValidate_GMHelperAPIBaseURL(t *testing.T) {
	validSecret := "dGVzdC1zZWNyZXQta2V5LTMyLWJ5dGVzLWxvbmchIQ=="

	// 1. Empty URL is valid (integration is optional)
	cfgEmptyURL := &Config{
		DatabaseURL:        "postgres://localhost/test",
		SMTPHost:           "smtp.example.com",
		SMTPFrom:           "test@example.com",
		HTTPPort:           8080,
		SMTPPort:           587,
		AuthSecret:         validSecret,
		GMHelperAPIBaseURL: "",
	}
	if err := cfgEmptyURL.Validate(); err != nil {
		t.Fatalf("expected valid config with empty GMHelperAPIBaseURL, got: %v", err)
	}

	// 2. Valid HTTP/HTTPS URL
	cfgValidURL := &Config{
		DatabaseURL:        "postgres://localhost/test",
		SMTPHost:           "smtp.example.com",
		SMTPFrom:           "test@example.com",
		HTTPPort:           8080,
		SMTPPort:           587,
		AuthSecret:         validSecret,
		GMHelperAPIBaseURL: "http://gmhelper-api:5000",
	}
	if err := cfgValidURL.Validate(); err != nil {
		t.Fatalf("expected valid config with valid GMHelperAPIBaseURL, got: %v", err)
	}

	// 3. Invalid URL scheme / missing host
	cfgInvalidURL := &Config{
		DatabaseURL:        "postgres://localhost/test",
		SMTPHost:           "smtp.example.com",
		SMTPFrom:           "test@example.com",
		HTTPPort:           8080,
		SMTPPort:           587,
		AuthSecret:         validSecret,
		GMHelperAPIBaseURL: "ftp://invalid-scheme",
	}
	if err := cfgInvalidURL.Validate(); err == nil {
		t.Fatal("expected error for invalid GMHelperAPIBaseURL scheme, got nil")
	}
}

func TestConfigValidate_ServiceAuthSettings(t *testing.T) {
	validSecret := "dGVzdC1zZWNyZXQta2V5LTMyLWJ5dGVzLWxvbmchIQ=="

	// 1. Defaults populated if empty
	cfgDefault := &Config{
		DatabaseURL: "postgres://localhost/test",
		SMTPHost:    "smtp.example.com",
		SMTPFrom:    "test@example.com",
		HTTPPort:    8080,
		SMTPPort:    587,
		AuthSecret:  validSecret,
		AuthIssuer:  "GMHelperAPI",
	}
	if err := cfgDefault.Validate(); err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}
	if cfgDefault.ServiceAuthIssuer != "GMHelperAPI" {
		t.Errorf("expected ServiceAuthIssuer 'GMHelperAPI', got '%s'", cfgDefault.ServiceAuthIssuer)
	}
	if cfgDefault.ServiceAuthAudience != "GMHelperClient" {
		t.Errorf("expected ServiceAuthAudience 'GMHelperClient', got '%s'", cfgDefault.ServiceAuthAudience)
	}

	// 2. Custom values preserved
	cfgCustom := &Config{
		DatabaseURL:         "postgres://localhost/test",
		SMTPHost:            "smtp.example.com",
		SMTPFrom:            "test@example.com",
		HTTPPort:            8080,
		SMTPPort:            587,
		AuthSecret:          validSecret,
		AuthIssuer:          "NotifyAPI",
		ServiceAuthIssuer:   "CustomIssuer",
		ServiceAuthAudience: "CustomAudience",
	}
	if err := cfgCustom.Validate(); err != nil {
		t.Fatalf("expected valid config with custom service auth, got: %v", err)
	}
	if cfgCustom.ServiceAuthIssuer != "CustomIssuer" {
		t.Errorf("expected ServiceAuthIssuer 'CustomIssuer', got '%s'", cfgCustom.ServiceAuthIssuer)
	}
	if cfgCustom.ServiceAuthAudience != "CustomAudience" {
		t.Errorf("expected ServiceAuthAudience 'CustomAudience', got '%s'", cfgCustom.ServiceAuthAudience)
	}
}

func TestConfigValidate_SchedulerSettings(t *testing.T) {
	validSecret := "dGVzdC1zZWNyZXQta2V5LTMyLWJ5dGVzLWxvbmchIQ=="

	// 1. Scheduler enabled with non-positive interval -> error
	cfgZeroInterval := &Config{
		DatabaseURL:        "postgres://localhost/test",
		SMTPHost:           "smtp.example.com",
		SMTPFrom:           "test@example.com",
		HTTPPort:           8080,
		SMTPPort:           587,
		AuthSecret:         validSecret,
		SchedulerEnabled:   true,
		SchedulerInterval:  0,
		SchedulerBatchSize: 10,
	}
	if err := cfgZeroInterval.Validate(); err == nil {
		t.Fatal("expected error when scheduler is enabled with zero interval, got nil")
	}

	// 2. Scheduler enabled with non-positive batch size -> error
	cfgZeroBatch := &Config{
		DatabaseURL:        "postgres://localhost/test",
		SMTPHost:           "smtp.example.com",
		SMTPFrom:           "test@example.com",
		HTTPPort:           8080,
		SMTPPort:           587,
		AuthSecret:         validSecret,
		SchedulerEnabled:   true,
		SchedulerInterval:  5 * time.Second,
		SchedulerBatchSize: 0,
	}
	if err := cfgZeroBatch.Validate(); err == nil {
		t.Fatal("expected error when scheduler is enabled with zero batch size, got nil")
	}

	// 3. Scheduler enabled with valid settings -> success
	cfgValid := &Config{
		DatabaseURL:        "postgres://localhost/test",
		SMTPHost:           "smtp.example.com",
		SMTPFrom:           "test@example.com",
		HTTPPort:           8080,
		SMTPPort:           587,
		AuthSecret:         validSecret,
		SchedulerEnabled:   true,
		SchedulerInterval:  5 * time.Second,
		SchedulerBatchSize: 10,
	}
	if err := cfgValid.Validate(); err != nil {
		t.Fatalf("expected valid config for scheduler, got: %v", err)
	}
}

func TestConfigValidate_CampaignWorkerSettings(t *testing.T) {
	validSecret := "dGVzdC1zZWNyZXQta2V5LTMyLWJ5dGVzLWxvbmchIQ=="

	// 1. Campaign worker enabled with non-positive interval -> error
	cfgZeroInterval := &Config{
		DatabaseURL:            "postgres://localhost/test",
		SMTPHost:               "smtp.example.com",
		SMTPFrom:               "test@example.com",
		HTTPPort:               8080,
		SMTPPort:               587,
		AuthSecret:             validSecret,
		CampaignWorkerEnabled:  true,
		CampaignWorkerInterval: 0,
	}
	if err := cfgZeroInterval.Validate(); err == nil {
		t.Fatal("expected error when campaign worker is enabled with zero interval, got nil")
	}

	// 2. Campaign worker enabled with non-positive batch size -> error
	cfgZeroBatch := &Config{
		DatabaseURL:             "postgres://localhost/test",
		SMTPHost:                "smtp.example.com",
		SMTPFrom:                "test@example.com",
		HTTPPort:                8080,
		SMTPPort:                587,
		AuthSecret:              validSecret,
		CampaignWorkerEnabled:   true,
		CampaignWorkerInterval:  5 * time.Second,
		CampaignWorkerBatchSize: 0,
	}
	if err := cfgZeroBatch.Validate(); err == nil {
		t.Fatal("expected error when campaign worker is enabled with zero batch size, got nil")
	}

	// 3. Campaign worker enabled with non-positive stale timeout -> error
	cfgZeroStale := &Config{
		DatabaseURL:                "postgres://localhost/test",
		SMTPHost:                   "smtp.example.com",
		SMTPFrom:                   "test@example.com",
		HTTPPort:                   8080,
		SMTPPort:                   587,
		AuthSecret:                 validSecret,
		CampaignWorkerEnabled:      true,
		CampaignWorkerInterval:     5 * time.Second,
		CampaignWorkerBatchSize:    20,
		CampaignWorkerStaleTimeout: 0,
	}
	if err := cfgZeroStale.Validate(); err == nil {
		t.Fatal("expected error when campaign worker is enabled with zero stale timeout, got nil")
	}

	// 4. Campaign worker enabled with valid settings -> success
	cfgValid := &Config{
		DatabaseURL:                "postgres://localhost/test",
		SMTPHost:                   "smtp.example.com",
		SMTPFrom:                   "test@example.com",
		HTTPPort:                   8080,
		SMTPPort:                   587,
		AuthSecret:                 validSecret,
		CampaignWorkerEnabled:      true,
		CampaignWorkerInterval:     5 * time.Second,
		CampaignWorkerBatchSize:    20,
		CampaignWorkerStaleTimeout: 5 * time.Minute,
	}
	if err := cfgValid.Validate(); err != nil {
		t.Fatalf("expected valid config for campaign worker, got: %v", err)
	}
}
