package catalogue

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		pool: pool,
	}
}

func (s *Service) CreateProduct(ctx context.Context, req CreateProductRequest) error {

	_, err := s.pool.Exec(ctx,
		`INSERT INTO products (tenant_id, name, code, unit, note) VALUES ($1, $2, $3, $4, $5)`,
		req.TenantID, req.Name, req.Code, req.Unit, req.Note)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDuplicateCode
		}
		slog.ErrorContext(ctx, "create product", "error", err, "tenant_id", req.TenantID, "code", req.Code)
		return err
	}

	return nil
}

func (s *Service) CreateLocation(ctx context.Context, req CreateLocationRequest) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO locations (tenant_id, name, code, address, note) VALUES ($1, $2, $3, $4, $5)`,
		req.TenantID, req.Name, req.Code, req.Address, req.Note)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDuplicateCode
		}
		slog.ErrorContext(ctx, "create location", "error", err, "tenant_id", req.TenantID, "code", req.Code)
		return err
	}

	return nil
}

func (s *Service) ListLocation(ctx context.Context, tenantId uuid.UUID) ([]Location, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, code, COALESCE(address, '') AS address, created_at, updated_at, COALESCE(note, '') AS note
		FROM locations
		WHERE tenant_id = $1 AND active`,
		tenantId,
	)

	if err != nil {
		slog.ErrorContext(ctx, "list locations", "error", err, "tenant_id", tenantId)
		return nil, err
	}

	locations, err := pgx.CollectRows(rows, pgx.RowToStructByName[Location])
	if err != nil {
		slog.ErrorContext(ctx, "list locations", "error", err, "tenant_id", tenantId)
		return nil, err
	}

	return locations, nil
}

func (s *Service) ListProduct(ctx context.Context, tenantId uuid.UUID) ([]Product, error) {

	rows, err := s.pool.Query(ctx,
		`SELECT id, name, code, unit, created_at, updated_at, COALESCE(note, '') AS note
		FROM products
		WHERE tenant_id = $1 AND active`,
		tenantId,
	)

	if err != nil {
		slog.ErrorContext(ctx, "list products", "error", err, "tenant_id", tenantId)
		return nil, err
	}

	products, err := pgx.CollectRows(rows, pgx.RowToStructByName[Product])
	if err != nil {
		slog.ErrorContext(ctx, "list products", "error", err, "tenant_id", tenantId)
		return nil, err
	}

	return products, nil
}

func (s *Service) UpdateProduct(ctx context.Context, req UpdateProductRequest) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE products SET name = $1, code = $2, unit = $3, note = $4, updated_at = now()
         WHERE tenant_id = $5 AND id = $6`,
		req.Name, req.Code, req.Unit, req.Note, req.TenantID, req.Id,
	)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDuplicateCode
		}
		slog.ErrorContext(ctx, "update product", "error", err, "tenant_id", req.TenantID, "id", req.Id)
		return err
	}

	if tag.RowsAffected() == 0 {
		return ErrProductNotFound
	}

	return nil
}

func (s *Service) UpdateLocation(ctx context.Context, req UpdateLocationRequest) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE locations SET name = $1, code = $2, address = $3, note = $4, updated_at = now()
         WHERE tenant_id = $5 AND id = $6`,
		req.Name, req.Code, req.Address, req.Note, req.TenantID, req.Id,
	)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDuplicateCode
		}
		slog.ErrorContext(ctx, "update location", "error", err, "tenant_id", req.TenantID, "id", req.Id)
		return err
	}

	if tag.RowsAffected() == 0 {
		return ErrLocationNotFound
	}

	return nil
}
