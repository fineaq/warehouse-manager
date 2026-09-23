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

var testSecret = []byte("test-only-jwt-secret-at-least-32-bytes-long")

// seedUser creates a tenant and a user whose password is known, and returns
// their IDs.
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

	const password = "thisisapassword"
	email, tenantID, userID := seedUser(t, pool, password)

	svc := auth.NewService(pool, testSecret)

	token, err := svc.Login(ctx, auth.LoginRequest{Email: email, Password: password})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

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

func TestValidateToken(t *testing.T) {
	svc := auth.NewService(nil, testSecret)

	userID, tenantID := uuid.New(), uuid.New()

	claim := validClaims(userID, tenantID, 15*time.Minute)
	newToken := createToken(t, jwt.SigningMethodHS256, testSecret, claim)

	claims, err := svc.ValidateToken(newToken)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	if claims.Subject != userID.String() {
		t.Fatalf("expected sub %s, got %s", userID, claims.Subject)
	}

	if claims.TenantID != tenantID.String() {
		t.Fatalf("expected tenant_id %s, got %s", tenantID, claims.TenantID)
	}
}

func TestValidateTokenInvalid(t *testing.T) {
	svc := auth.NewService(nil, testSecret)

	userID, tenantID := uuid.New(), uuid.New()
	valid := validClaims(userID, tenantID, time.Minute)
	expired := validClaims(userID, tenantID, -time.Minute)

	tests := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"garbage", "not-a-jwt"},
		{"wrong secret", createToken(t, jwt.SigningMethodHS256,
			[]byte("another-secret-at-least-32-bytes-long"), valid)},
		{"expired", createToken(t, jwt.SigningMethodHS256, testSecret, expired)},
		{"alg none", createToken(t, jwt.SigningMethodNone,
			jwt.UnsafeAllowNoneSignatureType, valid)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			claims, err := svc.ValidateToken(tc.token)
			if err == nil {
				t.Fatalf("expected an error, got claims %+v", claims)
			}
			if claims != nil {
				t.Fatalf("expected nil claims, got %+v", claims)
			}
		})
	}
}

func createToken(t *testing.T, method jwt.SigningMethod, secret any, claim auth.Claims) string {
	t.Helper()

	signed, err := jwt.NewWithClaims(method, claim).SignedString(secret)

	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	return signed
}

func validClaims(userID, tenantID uuid.UUID, ttl time.Duration) auth.Claims {
	return auth.Claims{
		TenantID: tenantID.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
		},
	}
}

func TestMain(m *testing.M) {
	testdb.Main(m)
}
