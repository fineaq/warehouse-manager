package catalogue

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type NewProductRequest struct {
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

type NewLocationRequest struct {
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

var (
	ErrDuplicateCode    = errors.New("duplicate product code")
	ErrProductNotFound  = errors.New("product not found")
	ErrLocationNotFound = errors.New("location not found")
)
