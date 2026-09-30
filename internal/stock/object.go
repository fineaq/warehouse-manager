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

type issueJSON struct {
	LocationID uuid.UUID       `json:"location_id" binding:"required"`
	ProductID  uuid.UUID       `json:"product_id" binding:"required"`
	Quantity   decimal.Decimal `json:"quantity" binding:"required"`
	Reason     Reason          `json:"reason" binding:"required"`
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

type IssueRequest struct {
	TenantID   uuid.UUID
	UserID     uuid.UUID
	LocationID uuid.UUID
	ProductID  uuid.UUID
	Quantity   decimal.Decimal
	Reason     Reason
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

type Reason string

const (
	ReasonReceive Reason = "receive"
	ReasonIssue   Reason = "issue"
	ReasonDispose Reason = "dispose"
)

var (
	ErrInvalidQuantity   = errors.New("quantity must be greater than zero")
	ErrProductNotFound   = errors.New("product not found")
	ErrLocationNotFound  = errors.New("location not found")
	ErrUserNotFound      = errors.New("user not found")
	ErrProductInactive   = errors.New("product is inactive")
	ErrLocationInactive  = errors.New("location is inactive")
	ErrReceiveRejected   = errors.New("product or location is inactive or missing")
	ErrInsufficientStock = errors.New("stock is insufficient")
	ErrInvalidReason     = errors.New("reason is not valid for an issue")
)
