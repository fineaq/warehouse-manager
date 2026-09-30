package stock

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"warehouse-manager/internal/db"

	"github.com/google/uuid"
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

func (s *Service) Issue(ctx context.Context, req IssueRequest) error {
	if req.Quantity.LessThanOrEqual(decimal.Zero) {
		return ErrInvalidQuantity
	}

	if req.Reason != ReasonIssue {
		return ErrInvalidReason
	}

	fn := func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`INSERT INTO stock_movements (tenant_id, location_id, product_id, user_id, quantity, reason, note)
        SELECT $1, $2, $3, $4, $5, $6, $7
        WHERE EXISTS (SELECT 1 FROM products  WHERE tenant_id=$1 AND id=$3 AND active)
         	  AND EXISTS (SELECT 1 FROM locations WHERE tenant_id=$1 AND id=$2 AND active)`,
			req.TenantID, req.LocationID, req.ProductID, req.UserID, req.Quantity.Neg(), req.Reason, req.Note,
		)
		if err != nil {
			if translated := translateFKViolation(err); translated != nil {
				return translated
			}
			return fmt.Errorf("insert movement: %w", err)
		}

		if tag.RowsAffected() == 0 {
			return rejectionReason(ctx, tx, req.TenantID, req.ProductID, req.LocationID)
		}

		var onHand decimal.Decimal
		if err := tx.QueryRow(ctx,
			`SELECT on_hand FROM stock_balances
		WHERE tenant_id=$1 AND location_id=$2 AND product_id=$3
		FOR UPDATE`,
			req.TenantID, req.LocationID, req.ProductID,
		).Scan(&onHand); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrInsufficientStock
			}
			return fmt.Errorf("lock balance: %w", err)
		}

		if onHand.LessThan(req.Quantity) {
			return ErrInsufficientStock
		}

		_, err = tx.Exec(ctx,
			`UPDATE stock_balances
    	SET on_hand = on_hand - $1, updated_at = now()
    	WHERE tenant_id=$2 AND location_id=$3 AND product_id=$4`,
			req.Quantity, req.TenantID, req.LocationID, req.ProductID,
		)
		if err != nil {
			return fmt.Errorf("upsert balance: %w", err)
		}

		return nil
	}

	if err := db.WithTx(ctx, s.pool, fn); err != nil {
		if !isRejection(err) {
			slog.ErrorContext(ctx, "issue stock",
				"error", err,
				"tenant_id", req.TenantID,
				"product_id", req.ProductID,
				"location_id", req.LocationID)
		}

		return err
	}

	return nil
}

func (s *Service) Receive(ctx context.Context, req ReceiveRequest) error {
	if req.Quantity.LessThanOrEqual(decimal.Zero) {
		return ErrInvalidQuantity
	}

	fn := func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`INSERT INTO stock_movements (tenant_id, location_id, product_id, user_id, quantity, reason, note)
         SELECT $1, $2, $3, $4, $5, 'receive', $6
         WHERE EXISTS (SELECT 1 FROM products  WHERE tenant_id=$1 AND id=$3 AND active)
           AND EXISTS (SELECT 1 FROM locations WHERE tenant_id=$1 AND id=$2 AND active)`,
			req.TenantID, req.LocationID, req.ProductID, req.UserID, req.Quantity, req.Note,
		)
		if err != nil {
			if translated := translateFKViolation(err); translated != nil {
				return translated
			}
			return fmt.Errorf("insert movement: %w", err)
		}

		if tag.RowsAffected() == 0 {
			return rejectionReason(ctx, tx, req.TenantID, req.ProductID, req.LocationID)
		}

		_, err = tx.Exec(ctx,
			`INSERT INTO stock_balances (tenant_id, location_id, product_id, on_hand)
         VALUES ($1, $2, $3, $4)
         ON CONFLICT (tenant_id, location_id, product_id)
         DO UPDATE SET on_hand = stock_balances.on_hand + EXCLUDED.on_hand, updated_at = now()`,
			req.TenantID, req.LocationID, req.ProductID, req.Quantity,
		)
		if err != nil {
			return fmt.Errorf("upsert balance: %w", err)
		}

		return nil
	}

	if err := db.WithTx(ctx, s.pool, fn); err != nil {
		if !isRejection(err) {
			slog.ErrorContext(ctx, "receive stock", "error", err,
				"tenant_id", req.TenantID,
				"product_id", req.ProductID,
				"location_id", req.LocationID)
		}

		return err
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
		slog.ErrorContext(ctx, "read on hand", "error", err,
			"tenant_id", req.TenantID, "product_id", req.ProductID, "location_id", req.LocationID)
		return decimal.Zero, err
	}

	return stockOnHand, nil

}

// isRejection reports whether err is the caller's mistake rather than a
// failure worth logging.
func isRejection(err error) bool {
	return errors.Is(err, ErrInvalidQuantity) ||
		errors.Is(err, ErrProductNotFound) ||
		errors.Is(err, ErrLocationNotFound) ||
		errors.Is(err, ErrProductInactive) ||
		errors.Is(err, ErrLocationInactive) ||
		errors.Is(err, ErrUserNotFound) ||
		errors.Is(err, ErrReceiveRejected) ||
		errors.Is(err, ErrInsufficientStock) ||
		errors.Is(err, ErrInvalidReason)
}

// rejectionReason says which precondition of the conditional insert failed.
// It runs in the same transaction, so it sees the rows the insert saw.
func rejectionReason(ctx context.Context, tx pgx.Tx, tenantID, productID, locationID uuid.UUID) error {
	var productActive, locationActive *bool

	err := tx.QueryRow(ctx,
		`SELECT (SELECT active FROM products  WHERE tenant_id=$1 AND id=$2),
		        (SELECT active FROM locations WHERE tenant_id=$1 AND id=$3)`,
		tenantID, productID, locationID,
	).Scan(&productActive, &locationActive)
	if err != nil {
		return fmt.Errorf("rejection reason: %w", err)
	}

	switch {
	case productActive == nil:
		return ErrProductNotFound
	case !*productActive:
		return ErrProductInactive
	case locationActive == nil:
		return ErrLocationNotFound
	case !*locationActive:
		return ErrLocationInactive
	}

	// Both look fine now, so the row changed under us between the insert and
	// this query.
	return ErrReceiveRejected
}

func translateFKViolation(err error) error {
	var pgErr *pgconn.PgError

	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		return nil
	}

	switch pgErr.ConstraintName {
	case "stock_movements_tenant_id_product_id_fkey":
		return fmt.Errorf("%w: %s", ErrProductNotFound, pgErr.ConstraintName)

	case "stock_movements_tenant_id_location_id_fkey":
		return fmt.Errorf("%w: %s", ErrLocationNotFound, pgErr.ConstraintName)

	case "stock_movements_tenant_id_user_id_fkey":
		return fmt.Errorf("%w: %s", ErrUserNotFound, pgErr.ConstraintName)
	}

	return nil
}
