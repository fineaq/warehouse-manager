// Command seed creates the single tenant and admin user that v1 starts with.

package main

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	"warehouse-manager/internal/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	adminName  = "admin"
	adminEmail = "admin@admin.com"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL not set")
	}

	password := os.Getenv("ADMIN_PASSWORD")
	if password == "" {
		password = "admin"
		log.Println("ADMIN_PASSWORD not set, using the default development password")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("hash password: %v", err)
	}

	var tenantID, userID uuid.UUID

	err = db.WithTx(ctx, pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`SELECT id FROM tenants WHERE email = $1`,
			adminEmail,
		).Scan(&tenantID)

		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx,
				`INSERT INTO tenants (name, email)
				 VALUES ($1, $2)
				 RETURNING id`,
				adminName, adminEmail,
			).Scan(&tenantID)
		}

		if err != nil {
			return err
		}

		if _, err := tx.Exec(ctx,
			`INSERT INTO users (tenant_id, name, email, password_hash)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (tenant_id, email) DO NOTHING`,
			tenantID, adminName, adminEmail, string(hash),
		); err != nil {
			return err
		}

		return tx.QueryRow(ctx,
			`SELECT id FROM users WHERE tenant_id = $1 AND email = $2`,
			tenantID, adminEmail,
		).Scan(&userID)
	})

	if err != nil {
		log.Fatalf("seed: %v", err)
	}

	log.Printf("seeded %s: tenant %s, user %s", adminEmail, tenantID, userID)
}
