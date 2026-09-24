package stock

import (
	"context"
	"errors"
	"fmt"
	"warehouse-manager/internal/db"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		pool: pool,
	}
}

func (s *Service) Receive(ctx context.Context, req ReceiveRequest) error {
	if req.Quantity.LessThanOrEqual(decimal.Zero) {
		return ErrInvalidQuantity
	}

	fn := func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO stock_movements(tenant_id, location_id, product_id, user_id, quantity, reason, note)
			VALUES ($1, $2, $3, $4, $5, 'receive', $6)`,
			req.TenantID, req.LocationID, req.ProductID, req.UserID, req.Quantity, req.Note,
		)

		if err != nil {
			return fmt.Errorf("insert stock movement: %w", err)
		}

		_, err = tx.Exec(ctx, `INSERT INTO stock_balances(tenant_id, location_id, product_id, on_hand)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (tenant_id, location_id, product_id)
			DO UPDATE SET on_hand = stock_balances.on_hand + EXCLUDED.on_hand, updated_at = now()`,
			req.TenantID, req.LocationID, req.ProductID, req.Quantity,
		)

		if err != nil {
			return fmt.Errorf("insert stock_balances: %w", err)
		}

		return nil
	}

	if err := db.WithTx(ctx, s.pool, fn); err != nil {
		return translateFKViolation(err)
	}

	return nil
}

func (s *Service) OnHand(ctx context.Context, req OnHandRequest) (decimal.Decimal, error) {
	var stockOnHand decimal.Decimal

	err := s.pool.QueryRow(ctx,
		`SELECT on_hand
     FROM stock_balances
     WHERE tenant_id=$1 AND product_id=$2 AND location_id=$3`,
		req.TenantID, req.ProductID, req.LocationID,
	).Scan(&stockOnHand)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return decimal.Zero, nil
		}
		return decimal.Zero, err
	}

	return stockOnHand, nil

}

func translateFKViolation(err error) error {
	var pgErr *pgconn.PgError

	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		return err
	}

	switch pgErr.ConstraintName {
	case "stock_movements_tenant_id_product_id_fkey",
		"stock_balances_tenant_id_product_id_fkey":
		return fmt.Errorf("%w: %s", ErrProductNotFound, pgErr.ConstraintName)

	case "stock_movements_tenant_id_location_id_fkey",
		"stock_balances_tenant_id_location_id_fkey":
		return fmt.Errorf("%w: %s", ErrLocationNotFound, pgErr.ConstraintName)

	case "stock_movements_tenant_id_user_id_fkey":
		return fmt.Errorf("%w: %s", ErrUserNotFound, pgErr.ConstraintName)
	}
	return err
}
