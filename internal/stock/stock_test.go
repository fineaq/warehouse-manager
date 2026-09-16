package stock_test

import (
	"context"
	"errors"
	"testing"
	"warehouse-manager/internal/stock"
	"warehouse-manager/internal/testdb"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

func seed(t *testing.T, pool *pgxpool.Pool) (tenantID, userID, locationID, productID uuid.UUID) {
	t.Helper()

	tenantID = uuid.New()
	userID = uuid.New()
	locationID = uuid.New()
	productID = uuid.New()

	ctx := context.Background()

	_, err := pool.Exec(ctx,
		`INSERT INTO tenants (id, name, email) VALUES ($1, 'test_tenant', 'test_tenant@test.co')`,
		tenantID)

	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, name, tenant_id, email, password_hash) VALUES ($1, 'test_user', $2, 'test_tenant@test.co', '12345')`,
		userID, tenantID)

	if err != nil {
		t.Fatalf("seed users: %v", err)
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO locations (id, name, tenant_id, code, address) VALUES ($1, 'test_location', $2, 'test_code', 'test_address')`,
		locationID, tenantID)

	if err != nil {
		t.Fatalf("seed locations: %v", err)
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO products (id, tenant_id, name, code, unit) VALUES ($1, $2, 'test_product', 'test_code', 'test_unit')`,
		productID, tenantID)

	if err != nil {
		t.Fatalf("seed products: %v", err)
	}

	return tenantID, userID, locationID, productID
}

func TestReceive(t *testing.T) {
	pool := testdb.New(t)

	ctx := context.Background()

	tenantID, userID, locationID, productID := seed(t, pool)

	svc := stock.NewService(pool)

	err := svc.Receive(ctx, stock.ReceiveRequest{
		TenantID:   tenantID,
		UserID:     userID,
		ProductID:  productID,
		LocationID: locationID,
		Quantity:   decimal.NewFromInt(20),
	})

	if err != nil {
		t.Fatalf("receive : %v", err)
	}

	var onhand decimal.Decimal

	err = pool.QueryRow(ctx,
		`SELECT on_hand FROM stock_balances
	WHERE tenant_id=$1 AND product_id=$2 AND location_id=$3`,
		tenantID, productID, locationID,
	).Scan(&onhand)

	if err != nil {
		t.Fatalf("query balance: %v", err)
	}

	if !onhand.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("expected on_hand 20 got %s", onhand)
	}

	var movementCount int
	var movementQty decimal.Decimal

	err = pool.QueryRow(ctx,
		`SELECT count(*), coalesce(sum(quantity), 0)
     FROM stock_movements
     WHERE tenant_id=$1 AND product_id=$2 AND location_id=$3`,
		tenantID, productID, locationID,
	).Scan(&movementCount, &movementQty)

	if err != nil {
		t.Fatalf("query movements: %v", err)
	}

	if movementCount != 1 {
		t.Fatalf("expected 1 movement, got %d", movementCount)
	}

	if !movementQty.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("expected movement qty 20, got %s", movementQty)
	}

}

func TestReceiveInvalid(t *testing.T) {
	pool := testdb.New(t)

	ctx := context.Background()

	tenantID, userID, locationID, productID := seed(t, pool)

	svc := stock.NewService(pool)

	tests := []struct {
		name    string
		req     stock.ReceiveRequest
		wantErr error
	}{
		{
			name: "zero quantity",
			req: stock.ReceiveRequest{
				TenantID:   tenantID,
				UserID:     userID,
				ProductID:  productID,
				LocationID: locationID,
				Quantity:   decimal.Zero,
			},
			wantErr: stock.ErrInvalidQuantity,
		},
		{
			name: "negative quantity",
			req: stock.ReceiveRequest{
				TenantID:   tenantID,
				UserID:     userID,
				ProductID:  productID,
				LocationID: locationID,
				Quantity:   decimal.NewFromInt(-5),
			},
			wantErr: stock.ErrInvalidQuantity,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.Receive(ctx, tc.req)

			if err == nil {
				t.Fatalf("expected error, got nil")
			}

			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected error %v, got %v", tc.wantErr, err)
			}
		})
	}

	var movementCount, balanceCount int

	err := pool.QueryRow(ctx, `SELECT count(*) FROM stock_movements`).Scan(&movementCount)
	if err != nil {
		t.Fatalf("query movements: %v", err)
	}

	if movementCount != 0 {
		t.Fatalf("expected 0 movements, got %d", movementCount)
	}

	err = pool.QueryRow(ctx, `SELECT count(*) FROM stock_balances`).Scan(&balanceCount)
	if err != nil {
		t.Fatalf("query balances: %v", err)
	}

	if balanceCount != 0 {
		t.Fatalf("expected 0 balances, got %d", balanceCount)
	}
}

func TestOnHand(t *testing.T) {
	pool := testdb.New(t)

	ctx := context.Background()

	tenantID, userID, locationID, productID := seed(t, pool)

	svc := stock.NewService(pool)

	receive := func(qty int64) {
		t.Helper()

		err := svc.Receive(ctx, stock.ReceiveRequest{
			TenantID:   tenantID,
			UserID:     userID,
			ProductID:  productID,
			LocationID: locationID,
			Quantity:   decimal.NewFromInt(qty),
		})

		if err != nil {
			t.Fatalf("receive %d: %v", qty, err)
		}
	}

	req := stock.OnHandRequest{
		TenantID:   tenantID,
		ProductID:  productID,
		LocationID: locationID,
	}

	receive(20)

	onHand, err := svc.OnHand(ctx, req)

	if err != nil {
		t.Fatalf("on hand: %v", err)
	}

	if !onHand.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("expected on hand 20, got %s", onHand)
	}

	// A second receipt at the same position must accumulate, not replace.
	receive(5)

	onHand, err = svc.OnHand(ctx, req)

	if err != nil {
		t.Fatalf("on hand after second receipt: %v", err)
	}

	if !onHand.Equal(decimal.NewFromInt(25)) {
		t.Fatalf("expected on hand 25, got %s", onHand)
	}
}
