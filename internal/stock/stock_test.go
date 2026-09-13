package stock_test

import (
	"context"
	"testing"
	"warehouse-manager/internal/stock"
	"warehouse-manager/internal/testdb"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestReceive(t *testing.T) {
	pool := testdb.New(t)

	tenantID := uuid.New()
	userID := uuid.New()
	locationID := uuid.New()
	productID := uuid.New()

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

	svc := stock.NewService(pool)

	err = svc.Receive(ctx, stock.ReceiveRequest{
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
