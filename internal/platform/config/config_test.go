package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func validConfig() Config {
	return Config{
		App: AppConfig{
			ServiceName:       "svc",
			Port:              "8080",
			Env:               EnvDevelopment,
			ReadHeaderTimeout: time.Second,
			ReadTimeout:       time.Second,
			WriteTimeout:      time.Second,
			IdleTimeout:       time.Second,
			ShutdownTimeout:   time.Second,
		},
		DB: DBConfig{
			Host: "localhost", Port: "5432", Name: "db", SSLMode: "disable",
			MaxConns: 10, MinConns: 1, MaxConnLifetime: time.Hour, MaxConnIdleTime: time.Minute,
			ConnectTimeout: time.Second, StatementTimeout: 500 * time.Millisecond,
		},
		Tracing:   TracingConfig{SampleRate: 1},
		OTLP:      OTLPConfig{Endpoint: "localhost:4317", Insecure: true},
		RateLimit: RateLimitConfig{RequestsPerMinute: 60},
		Auth:      AuthConfig{SessionTTL: 720 * time.Hour},
		Ingest:    IngestConfig{Interval: 5 * time.Minute, LiveInterval: 20 * time.Second, DetailsPerRun: 20},
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{"development ok", func(*Config) {}, ""},
		{"zero write timeout", func(c *Config) { c.App.WriteTimeout = 0 }, "APP_WRITE_TIMEOUT"},
		{"statement timeout exceeds write timeout", func(c *Config) { c.DB.StatementTimeout = 2 * time.Second }, "DB_STATEMENT_TIMEOUT must be less"},
		{"unknown env", func(c *Config) { c.App.Env = "prod" }, "APP_ENV"},
		{"trusted proxy not cidr", func(c *Config) { c.App.TrustedProxies = []string{"10.0.0.1"} }, "TRUSTED_PROXIES"},
		{"trusted proxy ok", func(c *Config) { c.App.TrustedProxies = []string{"10.0.0.0/8", "fd00::/8"} }, ""},
		{"production insecure", func(c *Config) {
			c.App.Env = EnvProduction
		}, "DB_SSL_MODE"},
		{"production ok", func(c *Config) {
			c.App.Env = EnvProduction
			c.DB.SSLMode = "verify-full"
			c.OTLP.Insecure = false
		}, ""},
		{"production allows explicit plaintext to local collector", func(c *Config) {
			c.App.Env = EnvProduction
			c.DB.SSLMode = "require"
			c.OTLP.Insecure = true
		}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(&cfg)
			err := cfg.Validate()

			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("unexpected error: %v", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Errorf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestSplitList(t *testing.T) {
	got := splitList(" a, ,b ,")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("splitList = %q", got)
	}
}

func setRequiredEnv(t *testing.T, env string) {
	t.Helper()
	for k, v := range map[string]string{
		"APP_ENV": env, "APP_SERVICE_NAME": "svc", "APP_PORT": "8080",
		"DB_HOST": "h", "DB_PORT": "5432", "DB_NAME": "n", "DB_SSL_MODE": "verify-full",
		"DB_MAX_CONNS": "5", "DB_MIN_CONNS": "1", "DB_MAX_CONN_LIFETIME": "1h", "DB_MAX_CONN_IDLE_TIME": "1m",
		"OTEL_EXPORTER_OTLP_ENDPOINT": "c:4317", "OTEL_EXPORTER_OTLP_INSECURE": "false",
	} {
		t.Setenv(k, v)
	}
}

func TestLoadWithoutDotEnv(t *testing.T) {
	t.Chdir(t.TempDir())
	setRequiredEnv(t, EnvDevelopment)

	if _, err := Load(); err != nil {
		t.Fatalf("Load without .env: %v", err)
	}
}

func TestLoadIgnoresDotEnvOutsideDevelopment(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("RATE_LIMIT_REQUESTS_PER_MINUTE=7\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	setRequiredEnv(t, EnvDevelopment)
	cfg, err := Load()
	if err != nil || cfg.RateLimit.RequestsPerMinute != 7 {
		t.Fatalf("development should read .env: cfg=%+v err=%v", cfg, err)
	}

	setRequiredEnv(t, EnvProduction)
	cfg, err = Load()
	if err != nil || cfg.RateLimit.RequestsPerMinute != 600 {
		t.Fatalf("production must ignore .env: rpm=%d err=%v", cfg.RateLimit.RequestsPerMinute, err)
	}
}
