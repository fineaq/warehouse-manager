package auth

import (
	"errors"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	AccessToken string    `json:"access_token"`
	UserID      uuid.UUID `json:"user_id"`
	TenantID    uuid.UUID `json:"tenant_id"`
}

type Claims struct {
	TenantID string `json:"tenant_id"`
	jwt.RegisteredClaims
}

var (
	ErrInvalidCredentials = errors.New("invalid credential")
)
