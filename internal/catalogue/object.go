package catalogue

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type CreateProductRequest struct {
	TenantID uuid.UUID
	Name     string
	Code     string
	Unit     string
	Note     string
}

type UpdateProductRequest struct {
	Id       uuid.UUID
	TenantID uuid.UUID
	Name     string
	Code     string
	Unit     string
	Note     string
}

type Product struct {
	Id        uuid.UUID `db:"id"`
	Name      string    `db:"name"`
	Code      string    `db:"code"`
	Unit      string    `db:"unit"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
	Note      string    `db:"note"`
}

type CreateLocationRequest struct {
	TenantID uuid.UUID
	Name     string
	Code     string
	Address  string
	Note     string
}

type UpdateLocationRequest struct {
	Id       uuid.UUID
	TenantID uuid.UUID
	Name     string
	Code     string
	Address  string
	Note     string
}

type Location struct {
	Id        uuid.UUID `db:"id"`
	Name      string    `db:"name"`
	Code      string    `db:"code"`
	Address   string    `db:"address"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
	Note      string    `db:"note"`
}

type createProductJSON struct {
	Name string `json:"name" binding:"required"`
	Code string `json:"code" binding:"required"`
	Unit string `json:"unit" binding:"required"`
	Note string `json:"note" binding:"required"`
}

type createLocationJSON struct {
	Name    string `json:"name" binding:"required"`
	Code    string `json:"code" binding:"required"`
	Address string `json:"address" binding:"required"`
	Note    string `json:"note" binding:"required"`
}

type productsJSON struct {
	Id        uuid.UUID `json:"id"`
	Name      string    `json:"name" binding:"required"`
	Code      string    `json:"code" binding:"required"`
	Unit      string    `json:"unit" binding:"required"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Note      string    `json:"note" binding:"required"`
}

type locationsJSON struct {
	Id        uuid.UUID `json:"id"`
	Name      string    `json:"name" binding:"required"`
	Code      string    `json:"code" binding:"required"`
	Address   string    `json:"address" binding:"required"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Note      string    `json:"note" binding:"required"`
}

var (
	ErrDuplicateCode    = errors.New("duplicate product code")
	ErrProductNotFound  = errors.New("product not found")
	ErrLocationNotFound = errors.New("location not found")
)
