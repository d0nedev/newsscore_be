// Command useradd creates an account. The password is read from stdin so it
// never lands in shell history or the process list:
//
//	printf '%s' "$PASSWORD" | go run ./cmd/useradd -email a@b.c -name "Nama" [-admin]
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/d0nedev/newsscore/internal/auth"
	"github.com/d0nedev/newsscore/internal/platform/config"
	"github.com/d0nedev/newsscore/internal/platform/database"
	db "github.com/d0nedev/newsscore/internal/platform/database/sqlc"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "useradd:", err)
		os.Exit(1)
	}
}

func run() error {
	email := flag.String("email", "", "login email")
	name := flag.String("name", "", "display name")
	admin := flag.Bool("admin", false, "grant the admin role")
	flag.Parse()

	if !strings.Contains(*email, "@") || strings.TrimSpace(*name) == "" {
		return fmt.Errorf("-email and -name are required")
	}

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return fmt.Errorf("read password from stdin: %w", err)
	}
	password := strings.TrimRight(line, "\r\n")
	if len(password) < 12 {
		return fmt.Errorf("password must be at least 12 characters")
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		return err
	}
	defer pool.Close()

	role := "user"
	if *admin {
		role = "admin"
	}
	id, err := db.New(pool).CreateUser(ctx, db.CreateUserParams{
		Email: strings.TrimSpace(*email), PasswordHash: hash, Name: strings.TrimSpace(*name), Role: role,
	})
	if err != nil {
		return err
	}

	fmt.Println("created user", id.String())
	return nil
}
