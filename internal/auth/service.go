package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	pool   *pgxpool.Pool
	secret []byte // the JWT signing secret
}

func NewService(pool *pgxpool.Pool, secret []byte) *Service {
	return &Service{pool: pool, secret: secret}
}

func (s *Service) Login(ctx context.Context, req LoginRequest) (string, error) {
	var storedHash, jwtToken string
	var tenantID, userID uuid.UUID

	if err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, password_hash FROM users WHERE email = $1`,
		req.Email,
	).Scan(&userID, &tenantID, &storedHash); err != nil {
		return "", ErrInvalidCredentials
	}

	err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(req.Password))
	if err != nil {
		// passwords don't match
		return "", ErrInvalidCredentials
	}

	jwtToken, err = s.issueToken(userID, tenantID)
	if err != nil {
		return "", err
	}

	return jwtToken, nil
}

func (s *Service) issueToken(userID, tenantID uuid.UUID) (string, error) {
	claims := Claims{
		TenantID: tenantID.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", err
	}

	return signed, nil
}

func (s *Service) ValidateToken(jwtToken string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(jwtToken, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return claims, nil
}
