package database

import (
	"context"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/d0nedev/newsscore/internal/platform/config"
)

func TestIntegrationPoolAppliesStatementTimeout(t *testing.T) {
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	password, _ := u.User.Password()

	pool, err := NewPostgresPool(context.Background(), config.DBConfig{
		Host: u.Hostname(), Port: u.Port(), User: u.User.Username(), Password: password,
		Name: u.Path[1:], SSLMode: u.Query().Get("sslmode"),
		MaxConns: 2, MaxConnLifetime: time.Minute, MaxConnIdleTime: time.Minute,
		ConnectTimeout: 5 * time.Second, StatementTimeout: 1234 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var setting string
	if err := pool.QueryRow(context.Background(), "SHOW statement_timeout").Scan(&setting); err != nil {
		t.Fatal(err)
	}
	if setting != "1234ms" {
		t.Errorf("statement_timeout = %q, want 1234ms", setting)
	}

	_, err = pool.Exec(context.Background(), "SELECT pg_sleep(2)")
	if err == nil {
		t.Error("query longer than statement_timeout was not cancelled")
	}
}
