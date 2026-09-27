package config

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/netip"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const (
	EnvDevelopment = "development"
	EnvStaging     = "staging"
	EnvProduction  = "production"
)

type Config struct {
	App       AppConfig
	DB        DBConfig
	Tracing   TracingConfig
	OTLP      OTLPConfig
	Auth      AuthConfig
	RateLimit RateLimitConfig
	Ingest    IngestConfig
}

type AppConfig struct {
	ServiceName        string
	Port               string
	Env                string
	LogLevel           slog.Level
	ReadHeaderTimeout  time.Duration
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	IdleTimeout        time.Duration
	ShutdownDrainDelay time.Duration
	ShutdownTimeout    time.Duration
	TrustedProxies     []string
}

type DBConfig struct {
	Host             string
	Port             string
	User             string
	Password         string
	Name             string
	SSLMode          string
	MaxConns         int32
	MinConns         int32
	MaxConnLifetime  time.Duration
	MaxConnIdleTime  time.Duration
	ConnectTimeout   time.Duration
	StatementTimeout time.Duration
}

type OTLPConfig struct {
	Endpoint string
	Insecure bool
}

type TracingConfig struct {
	SampleRate float64
}

type AuthConfig struct {
	SessionTTL     time.Duration
	CookieSecure   bool
	AllowedOrigins []string // CORS origins allowed to send the session cookie
}

type IngestConfig struct {
	Interval      time.Duration
	LiveInterval  time.Duration
	DetailsPerRun int
	FSign         string
}

type RateLimitConfig struct {
	RequestsPerMinute int
}

func Load() (*Config, error) {
	v := viper.New()
	v.AutomaticEnv()

	// .env is a developer convenience only; deployed environments must get
	// configuration (and secrets) from the real environment.
	if env := os.Getenv("APP_ENV"); env == "" || env == EnvDevelopment {
		v.SetConfigFile(".env")
		v.SetConfigType("env")

		if err := v.ReadInConfig(); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("read .env: %w", err)
		}
	}

	v.SetDefault("APP_ENV", EnvDevelopment)
	v.SetDefault("LOG_LEVEL", "INFO")
	v.SetDefault("APP_READ_HEADER_TIMEOUT", "5s")
	v.SetDefault("APP_READ_TIMEOUT", "10s")
	v.SetDefault("APP_WRITE_TIMEOUT", "10s")
	v.SetDefault("APP_IDLE_TIMEOUT", "60s")
	v.SetDefault("APP_SHUTDOWN_DRAIN_DELAY", "5s")
	v.SetDefault("APP_SHUTDOWN_TIMEOUT", "10s")
	v.SetDefault("DB_CONNECT_TIMEOUT", "30s")
	v.SetDefault("DB_STATEMENT_TIMEOUT", "5s")
	v.SetDefault("DB_SSL_MODE", "require")
	v.SetDefault("OTEL_TRACE_SAMPLE_RATE", 0.1)
	v.SetDefault("RATE_LIMIT_REQUESTS_PER_MINUTE", 600)
	v.SetDefault("SESSION_TTL", "720h")
	v.SetDefault("INGEST_INTERVAL", "5m")
	v.SetDefault("INGEST_DETAILS_PER_RUN", 20)
	v.SetDefault("INGEST_LIVE_INTERVAL", "20s")

	env := v.GetString("APP_ENV")

	// Plaintext OTLP is only a sane default on a developer machine. Production may
	// still opt in explicitly for a collector on the same private network.
	v.SetDefault("OTEL_EXPORTER_OTLP_INSECURE", env == EnvDevelopment)

	logLevel, err := parseLogLevel(v.GetString("LOG_LEVEL"))
	if err != nil {
		return nil, err
	}

	config := &Config{
		App: AppConfig{
			ServiceName:        v.GetString("APP_SERVICE_NAME"),
			Port:               v.GetString("APP_PORT"),
			Env:                env,
			LogLevel:           logLevel,
			ReadHeaderTimeout:  v.GetDuration("APP_READ_HEADER_TIMEOUT"),
			ReadTimeout:        v.GetDuration("APP_READ_TIMEOUT"),
			WriteTimeout:       v.GetDuration("APP_WRITE_TIMEOUT"),
			IdleTimeout:        v.GetDuration("APP_IDLE_TIMEOUT"),
			ShutdownDrainDelay: v.GetDuration("APP_SHUTDOWN_DRAIN_DELAY"),
			ShutdownTimeout:    v.GetDuration("APP_SHUTDOWN_TIMEOUT"),
			TrustedProxies:     splitList(v.GetString("TRUSTED_PROXIES")),
		},
		DB: DBConfig{
			Host:             v.GetString("DB_HOST"),
			Port:             v.GetString("DB_PORT"),
			User:             v.GetString("DB_USER"),
			Password:         v.GetString("DB_PASSWORD"),
			Name:             v.GetString("DB_NAME"),
			SSLMode:          v.GetString("DB_SSL_MODE"),
			MaxConns:         int32(v.GetInt("DB_MAX_CONNS")),
			MinConns:         int32(v.GetInt("DB_MIN_CONNS")),
			MaxConnLifetime:  v.GetDuration("DB_MAX_CONN_LIFETIME"),
			MaxConnIdleTime:  v.GetDuration("DB_MAX_CONN_IDLE_TIME"),
			ConnectTimeout:   v.GetDuration("DB_CONNECT_TIMEOUT"),
			StatementTimeout: v.GetDuration("DB_STATEMENT_TIMEOUT"),
		},
		Tracing: TracingConfig{
			SampleRate: v.GetFloat64("OTEL_TRACE_SAMPLE_RATE"),
		},
		OTLP: OTLPConfig{
			Endpoint: v.GetString("OTEL_EXPORTER_OTLP_ENDPOINT"),
			Insecure: v.GetBool("OTEL_EXPORTER_OTLP_INSECURE"),
		},
		Auth: AuthConfig{
			SessionTTL:     v.GetDuration("SESSION_TTL"),
			CookieSecure:   env != EnvDevelopment, // plain http://localhost needs non-Secure cookies
			AllowedOrigins: splitList(v.GetString("CORS_ALLOWED_ORIGINS")),
		},
		RateLimit: RateLimitConfig{
			RequestsPerMinute: v.GetInt("RATE_LIMIT_REQUESTS_PER_MINUTE"),
		},
		Ingest: IngestConfig{
			Interval:      v.GetDuration("INGEST_INTERVAL"),
			LiveInterval:  v.GetDuration("INGEST_LIVE_INTERVAL"),
			DetailsPerRun: v.GetInt("INGEST_DETAILS_PER_RUN"),
			FSign:         v.GetString("FLASHSCORE_FSIGN"),
		},
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

func (c *Config) Validate() error {
	var errs []error

	require := func(ok bool, msg string) {
		if !ok {
			errs = append(errs, errors.New(msg))
		}
	}

	require(slices.Contains([]string{EnvDevelopment, EnvStaging, EnvProduction}, c.App.Env),
		"APP_ENV must be one of development, staging, production")
	require(c.App.ServiceName != "", "APP_SERVICE_NAME is required")
	require(c.App.Port != "", "APP_PORT is required")
	require(c.App.ReadHeaderTimeout > 0, "APP_READ_HEADER_TIMEOUT must be greater than 0")
	require(c.App.ReadTimeout > 0, "APP_READ_TIMEOUT must be greater than 0")
	require(c.App.WriteTimeout > 0, "APP_WRITE_TIMEOUT must be greater than 0")
	require(c.App.IdleTimeout > 0, "APP_IDLE_TIMEOUT must be greater than 0")
	require(c.App.ShutdownDrainDelay >= 0, "APP_SHUTDOWN_DRAIN_DELAY must not be negative")
	require(c.App.ShutdownTimeout > 0, "APP_SHUTDOWN_TIMEOUT must be greater than 0")

	for _, proxy := range c.App.TrustedProxies {
		_, err := netip.ParsePrefix(proxy)
		require(err == nil, "TRUSTED_PROXIES must be CIDR prefixes, got "+proxy)
	}

	require(c.DB.Host != "", "DB_HOST is required")
	require(c.DB.Port != "", "DB_PORT is required")
	require(c.DB.Name != "", "DB_NAME is required")
	require(c.DB.MaxConns > 0, "DB_MAX_CONNS must be greater than 0")
	require(c.DB.MinConns >= 0, "DB_MIN_CONNS must be greater than or equal to 0")
	require(c.DB.MinConns <= c.DB.MaxConns, "DB_MIN_CONNS must not be greater than DB_MAX_CONNS")
	require(c.DB.MaxConnLifetime > 0, "DB_MAX_CONN_LIFETIME must be greater than 0")
	require(c.DB.MaxConnIdleTime > 0, "DB_MAX_CONN_IDLE_TIME must be greater than 0")
	require(c.DB.ConnectTimeout > 0, "DB_CONNECT_TIMEOUT must be greater than 0")
	require(c.DB.StatementTimeout > 0, "DB_STATEMENT_TIMEOUT must be greater than 0")
	require(c.DB.StatementTimeout < c.App.WriteTimeout,
		"DB_STATEMENT_TIMEOUT must be less than APP_WRITE_TIMEOUT so queries end before the response deadline")

	require(c.Tracing.SampleRate >= 0 && c.Tracing.SampleRate <= 1,
		"OTEL_TRACE_SAMPLE_RATE must be between 0 and 1")
	require(c.OTLP.Endpoint != "", "OTEL_EXPORTER_OTLP_ENDPOINT is required")

	require(c.RateLimit.RequestsPerMinute > 0, "RATE_LIMIT_REQUESTS_PER_MINUTE must be greater than 0")
	require(c.Ingest.Interval >= time.Minute, "INGEST_INTERVAL must be at least 1m")
	require(c.Auth.SessionTTL >= time.Hour, "SESSION_TTL must be at least 1h")
	for _, o := range c.Auth.AllowedOrigins {
		require(o != "*" && (strings.HasPrefix(o, "https://") || strings.HasPrefix(o, "http://")),
			"CORS_ALLOWED_ORIGINS must list exact origins like https://app.example.com, not *")
	}
	require(c.Ingest.LiveInterval >= 10*time.Second, "INGEST_LIVE_INTERVAL must be at least 10s")
	require(c.Ingest.DetailsPerRun >= 0, "INGEST_DETAILS_PER_RUN must not be negative")

	if c.App.Env == EnvProduction {
		require(slices.Contains([]string{"require", "verify-ca", "verify-full"}, c.DB.SSLMode),
			"DB_SSL_MODE must be require, verify-ca, or verify-full in production")
	}

	return errors.Join(errs...)
}

func parseLogLevel(value string) (slog.Level, error) {
	switch strings.ToUpper(value) {
	case "DEBUG":
		return slog.LevelDebug, nil
	case "INFO":
		return slog.LevelInfo, nil
	case "WARN":
		return slog.LevelWarn, nil
	case "ERROR":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid LOG_LEVEL: %s", value)
	}
}

func splitList(value string) []string {
	var out []string
	for item := range strings.SplitSeq(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
