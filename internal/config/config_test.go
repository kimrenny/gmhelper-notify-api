package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfigLoad_Success(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Setenv("SMTP_HOST", "smtp.example.com")
	os.Setenv("SMTP_FROM", "test@example.com")
	os.Setenv("HTTP_PORT", "9090")
	os.Setenv("SMTP_PORT", "2525")
	defer func() {
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("SMTP_HOST")
		os.Unsetenv("SMTP_FROM")
		os.Unsetenv("HTTP_PORT")
		os.Unsetenv("SMTP_PORT")
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
		AuthSecret:  "gmhelper-secret-key-change-in-production",
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

	// Explicit production secret allowed
	cfgValidProd := &Config{
		Env:         "production",
		DatabaseURL: "postgres://localhost/test",
		SMTPHost:    "smtp.example.com",
		SMTPFrom:    "test@example.com",
		HTTPPort:    8080,
		SMTPPort:    587,
		AuthSecret:  "super-secure-production-secret-value-32-chars",
	}
	if err := cfgValidProd.Validate(); err != nil {
		t.Fatalf("expected valid production config, got: %v", err)
	}
}

func TestConfigValidate_WorkerSettings(t *testing.T) {
	// Worker enabled with zero/negative interval -> error
	cfgInvalidInterval := &Config{
		DatabaseURL:    "postgres://localhost/test",
		SMTPHost:       "smtp.example.com",
		SMTPFrom:       "test@example.com",
		HTTPPort:       8080,
		SMTPPort:       587,
		AuthSecret:     "secret-32-chars-long-here!!!!!!",
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
		AuthSecret:     "secret-32-chars-long-here!!!!!!",
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
		AuthSecret:         "secret-32-chars-long-here!!!!!!",
		WorkerEnabled:      true,
		WorkerInterval:     5 * time.Second,
		WorkerStaleTimeout: 0,
	}
	if err := cfgInvalidStaleTimeout.Validate(); err == nil {
		t.Fatal("expected error when worker is enabled with zero stale timeout, got nil")
	}

	// Worker enabled with valid settings -> ok
	cfgValidWorker := &Config{
		DatabaseURL:        "postgres://localhost/test",
		SMTPHost:           "smtp.example.com",
		SMTPFrom:           "test@example.com",
		HTTPPort:           8080,
		SMTPPort:           587,
		AuthSecret:         "secret-32-chars-long-here!!!!!!",
		WorkerEnabled:      true,
		WorkerInterval:     5 * time.Second,
		WorkerStaleTimeout: 5 * time.Minute,
	}
	if err := cfgValidWorker.Validate(); err != nil {
		t.Fatalf("expected valid config for enabled worker with positive interval and stale timeout, got: %v", err)
	}
}
