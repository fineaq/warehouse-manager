package stock

import (
	"errors"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type ReceiveRequest struct {
	TenantID   uuid.UUID
	LocationID uuid.UUID
	ProductID  uuid.UUID
	UserID     uuid.UUID
	Quantity   decimal.Decimal
	Note       string
}

type OnHandRequest struct {
	TenantID   uuid.UUID
	LocationID uuid.UUID
	ProductID  uuid.UUID
}

type OnHandResponse struct {
	Quantity decimal.Decimal `json:"quantity"`
}

var (
	ErrInvalidQuantity = errors.New("quantity must be greater than zero")
	ErrProductNotFound = errors.New("product not found")
)
