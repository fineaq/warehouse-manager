package stock

import (
	"errors"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type receiveJSON struct {
	LocationID uuid.UUID       `json:"location_id" binding:"required"`
	ProductID  uuid.UUID       `json:"product_id" binding:"required"`
	Quantity   decimal.Decimal `json:"quantity" binding:"required"`
	Note       string          `json:"note"`
}

type ReceiveRequest struct {
	TenantID   uuid.UUID
	UserID     uuid.UUID
	LocationID uuid.UUID
	ProductID  uuid.UUID
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
	ErrInvalidQuantity  = errors.New("quantity must be greater than zero")
	ErrProductNotFound  = errors.New("product not found")
	ErrLocationNotFound = errors.New("location not found")
	ErrUserNotFound     = errors.New("user not found")
)
