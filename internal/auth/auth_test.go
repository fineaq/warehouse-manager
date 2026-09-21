package auth_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
	"warehouse-manager/internal/auth"
	"warehouse-manager/internal/testdb"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// A fixed secret keeps the test independent of anyone's .env.
var testSecret = []byte("test-only-jwt-secret-at-least-32-bytes-long")

// seedUser creates a tenant and a user whose password is known, and returns
// their IDs. The email is unique per call: login looks users up by email
// alone, so two users sharing an email would make the lookup ambiguous.
func seedUser(t *testing.T, pool *pgxpool.Pool, password string) (email string, tenantID, userID uuid.UUID) {
	t.Helper()

	ctx := context.Background()
	email = fmt.Sprintf("user-%s@test.co", uuid.New())

	// MinCost keeps the test fast; production cost is irrelevant here.
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	err = pool.QueryRow(ctx,
		`INSERT INTO tenants (name, email) VALUES ('test_tenant', $1) RETURNING id`,
		email,
	).Scan(&tenantID)
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}

	err = pool.QueryRow(ctx,
		`INSERT INTO users (tenant_id, name, email, password_hash)
		 VALUES ($1, 'test_user', $2, $3)
		 RETURNING id`,
		tenantID, email, string(hash),
	).Scan(&userID)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	return email, tenantID, userID
}

func TestLogin(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	const password = "correct-horse-battery-staple"
	email, tenantID, userID := seedUser(t, pool, password)

	svc := auth.NewService(pool, testSecret)

	token, err := svc.Login(ctx, auth.LoginRequest{Email: email, Password: password})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	// A non-empty string proves nothing. Parse it back with the same secret
	// and check it identifies the seeded user.
	claims := &auth.Claims{}
	_, err = jwt.ParseWithClaims(token, claims,
		func(*jwt.Token) (any, error) { return testSecret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
	)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}

	if claims.Subject != userID.String() {
		t.Fatalf("expected sub %s, got %s", userID, claims.Subject)
	}

	if claims.TenantID != tenantID.String() {
		t.Fatalf("expected tenant_id %s, got %s", tenantID, claims.TenantID)
	}

	if claims.ExpiresAt == nil || !claims.ExpiresAt.After(time.Now()) {
		t.Fatalf("expected an expiry in the future, got %v", claims.ExpiresAt)
	}
}

func TestLoginInvalid(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	const password = "correct-horse-battery-staple"
	email, _, _ := seedUser(t, pool, password)

	svc := auth.NewService(pool, testSecret)

	// Both cases must fail the same way: the caller must not be able to tell
	// an unknown email from a wrong password.
	tests := []struct {
		name string
		req  auth.LoginRequest
	}{
		{
			name: "wrong password",
			req:  auth.LoginRequest{Email: email, Password: "wrong-password"},
		},
		{
			name: "unknown email",
			req:  auth.LoginRequest{Email: "nobody@test.co", Password: password},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			token, err := svc.Login(ctx, tc.req)

			if !errors.Is(err, auth.ErrInvalidCredentials) {
				t.Fatalf("expected ErrInvalidCredentials, got %v", err)
			}

			if token != "" {
				t.Fatalf("expected no token, got %q", token)
			}
		})
	}
}

func TestMain(m *testing.M) {
	testdb.Main(m)
}
