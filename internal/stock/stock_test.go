package stock_test

import (
	"context"
	"errors"
	"testing"
	"warehouse-manager/internal/stock"
	"warehouse-manager/internal/testdb"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestReceive(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	acc := testdb.SeedAccount(t, pool)
	svc := stock.NewService(pool)

	tests := []struct {
		name          string
		quantity      int64
		wantOnHand    int64
		wantMovements int
	}{
		{
			name:          "first receipt",
			quantity:      20,
			wantOnHand:    20,
			wantMovements: 1,
		},
		{
			name:          "second receipt accumulates",
			quantity:      5,
			wantOnHand:    25,
			wantMovements: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.Receive(ctx, stock.ReceiveRequest{
				TenantID:   acc.TenantID,
				UserID:     acc.UserID,
				ProductID:  acc.ProductID,
				LocationID: acc.LocationID,
				Quantity:   decimal.NewFromInt(tc.quantity),
			})

			if err != nil {
				t.Fatalf("receive: %v", err)
			}

			var onhand decimal.Decimal

			err = pool.QueryRow(ctx,
				`SELECT on_hand FROM stock_balances
			WHERE tenant_id=$1 AND product_id=$2 AND location_id=$3`,
				acc.TenantID, acc.ProductID, acc.LocationID,
			).Scan(&onhand)

			if err != nil {
				t.Fatalf("query balance: %v", err)
			}

			if !onhand.Equal(decimal.NewFromInt(tc.wantOnHand)) {
				t.Fatalf("expected on_hand %d, got %s", tc.wantOnHand, onhand)
			}

			var movementCount int
			var movementQty decimal.Decimal

			err = pool.QueryRow(ctx,
				`SELECT count(*), coalesce(sum(quantity), 0)
			 FROM stock_movements
			 WHERE tenant_id=$1 AND product_id=$2 AND location_id=$3`,
				acc.TenantID, acc.ProductID, acc.LocationID,
			).Scan(&movementCount, &movementQty)

			if err != nil {
				t.Fatalf("query movements: %v", err)
			}

			if movementCount != tc.wantMovements {
				t.Fatalf("expected %d movements, got %d", tc.wantMovements, movementCount)
			}

			// The movements are the ledger; their sum must always match the balance.
			if !movementQty.Equal(onhand) {
				t.Fatalf("expected movements to sum to on_hand %s, got %s", onhand, movementQty)
			}
		})
	}
}

func TestReceiveInvalid(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	acc := testdb.SeedAccount(t, pool)
	svc := stock.NewService(pool)

	var inactiveProductID, inactiveLocationID uuid.UUID

	if err := pool.QueryRow(ctx,
		`INSERT INTO products (tenant_id, name, code, unit, active)
		 VALUES ($1, 'inactive_product', 'inactive_product_code', 'pcs', false) RETURNING id`,
		acc.TenantID).Scan(&inactiveProductID); err != nil {
		t.Fatalf("seed inactive product: %v", err)
	}

	if err := pool.QueryRow(ctx,
		`INSERT INTO locations (tenant_id, name, code, active)
		 VALUES ($1, 'inactive_location', 'inactive_location_code', false) RETURNING id`,
		acc.TenantID).Scan(&inactiveLocationID); err != nil {
		t.Fatalf("seed inactive location: %v", err)
	}

	tests := []struct {
		name    string
		req     stock.ReceiveRequest
		wantErr error
	}{
		{
			name: "inactive product",
			req: stock.ReceiveRequest{
				TenantID:   acc.TenantID,
				UserID:     acc.UserID,
				ProductID:  inactiveProductID,
				LocationID: acc.LocationID,
				Quantity:   decimal.NewFromInt(20),
			},
			wantErr: stock.ErrProductInactive,
		},
		{
			name: "inactive location",
			req: stock.ReceiveRequest{
				TenantID:   acc.TenantID,
				UserID:     acc.UserID,
				ProductID:  acc.ProductID,
				LocationID: inactiveLocationID,
				Quantity:   decimal.NewFromInt(20),
			},
			wantErr: stock.ErrLocationInactive,
		},
		{
			name: "zero quantity",
			req: stock.ReceiveRequest{
				TenantID:   acc.TenantID,
				UserID:     acc.UserID,
				ProductID:  acc.ProductID,
				LocationID: acc.LocationID,
				Quantity:   decimal.Zero,
			},
			wantErr: stock.ErrInvalidQuantity,
		},
		{
			name: "negative quantity",
			req: stock.ReceiveRequest{
				TenantID:   acc.TenantID,
				UserID:     acc.UserID,
				ProductID:  acc.ProductID,
				LocationID: acc.LocationID,
				Quantity:   decimal.NewFromInt(-5),
			},
			wantErr: stock.ErrInvalidQuantity,
		},
		{
			name: "unknown product",
			req: stock.ReceiveRequest{
				TenantID:   acc.TenantID,
				UserID:     acc.UserID,
				ProductID:  uuid.New(),
				LocationID: acc.LocationID,
				Quantity:   decimal.NewFromInt(20),
			},
			wantErr: stock.ErrProductNotFound,
		},
		{
			name: "unknown location",
			req: stock.ReceiveRequest{
				TenantID:   acc.TenantID,
				UserID:     acc.UserID,
				ProductID:  acc.ProductID,
				LocationID: uuid.New(),
				Quantity:   decimal.NewFromInt(20),
			},
			wantErr: stock.ErrLocationNotFound,
		},
		{
			name: "unknown user",
			req: stock.ReceiveRequest{
				TenantID:   acc.TenantID,
				UserID:     uuid.New(),
				ProductID:  acc.ProductID,
				LocationID: acc.LocationID,
				Quantity:   decimal.NewFromInt(20),
			},
			wantErr: stock.ErrUserNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.Receive(ctx, tc.req)

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}

	var movementCount, balanceCount int

	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM stock_movements WHERE tenant_id=$1`,
		acc.TenantID).Scan(&movementCount); err != nil {
		t.Fatalf("query movements: %v", err)
	}

	if movementCount != 0 {
		t.Fatalf("expected 0 movements, got %d", movementCount)
	}

	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM stock_balances WHERE tenant_id=$1`,
		acc.TenantID).Scan(&balanceCount); err != nil {
		t.Fatalf("query balances: %v", err)
	}

	if balanceCount != 0 {
		t.Fatalf("expected 0 balances, got %d", balanceCount)
	}
}

func TestOnHand(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	acc := testdb.SeedAccount(t, pool)
	svc := stock.NewService(pool)

	receive := func(qty int64) {
		t.Helper()

		err := svc.Receive(ctx, stock.ReceiveRequest{
			TenantID:   acc.TenantID,
			UserID:     acc.UserID,
			ProductID:  acc.ProductID,
			LocationID: acc.LocationID,
			Quantity:   decimal.NewFromInt(qty),
		})

		if err != nil {
			t.Fatalf("receive %d: %v", qty, err)
		}
	}

	req := stock.OnHandRequest{
		TenantID:   acc.TenantID,
		ProductID:  acc.ProductID,
		LocationID: acc.LocationID,
	}

	tests := []struct {
		name         string
		receiveFirst int64
		want         int64
	}{
		{
			name:         "no stock yet",
			receiveFirst: 0,
			want:         0,
		},
		{
			name:         "after first receipt",
			receiveFirst: 20,
			want:         20,
		},
		{
			// A second receipt at the same position accumulates, not replaces.
			name:         "second receipt accumulates",
			receiveFirst: 5,
			want:         25,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.receiveFirst != 0 {
				receive(tc.receiveFirst)
			}

			onHand, err := svc.OnHand(ctx, req)

			if err != nil {
				t.Fatalf("on hand: %v", err)
			}

			if !onHand.Equal(decimal.NewFromInt(tc.want)) {
				t.Fatalf("expected on hand %d, got %s", tc.want, onHand)
			}
		})
	}
}

func TestIssue(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	acc := testdb.SeedAccount(t, pool)
	svc := stock.NewService(pool)

	err := svc.Receive(ctx, stock.ReceiveRequest{
		TenantID:   acc.TenantID,
		UserID:     acc.UserID,
		ProductID:  acc.ProductID,
		LocationID: acc.LocationID,
		Quantity:   decimal.NewFromInt(20),
	})
	if err != nil {
		t.Fatalf("receive: %v", err)
	}

	err = svc.Issue(ctx, stock.IssueRequest{
		TenantID:   acc.TenantID,
		UserID:     acc.UserID,
		ProductID:  acc.ProductID,
		LocationID: acc.LocationID,
		Quantity:   decimal.NewFromInt(5),
		Reason:     stock.ReasonIssue,
		Note:       "sold to a customer",
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	onHand, err := svc.OnHand(ctx, stock.OnHandRequest{
		TenantID:   acc.TenantID,
		ProductID:  acc.ProductID,
		LocationID: acc.LocationID,
	})
	if err != nil {
		t.Fatalf("on hand: %v", err)
	}

	if !onHand.Equal(decimal.NewFromInt(15)) {
		t.Fatalf("expected on hand 15, got %s", onHand)
	}

	// Stock leaving is recorded as a negative movement.
	var issued decimal.Decimal

	err = pool.QueryRow(ctx,
		`SELECT quantity FROM stock_movements
		 WHERE tenant_id=$1 AND product_id=$2 AND location_id=$3 AND reason='issue'`,
		acc.TenantID, acc.ProductID, acc.LocationID,
	).Scan(&issued)
	if err != nil {
		t.Fatalf("query issue movement: %v", err)
	}

	if !issued.Equal(decimal.NewFromInt(-5)) {
		t.Fatalf("expected movement -5, got %s", issued)
	}

	// On hand must equal the sum of the movements behind it.
	var movementSum decimal.Decimal

	err = pool.QueryRow(ctx,
		`SELECT coalesce(sum(quantity), 0) FROM stock_movements
		 WHERE tenant_id=$1 AND product_id=$2 AND location_id=$3`,
		acc.TenantID, acc.ProductID, acc.LocationID,
	).Scan(&movementSum)
	if err != nil {
		t.Fatalf("sum movements: %v", err)
	}

	if !movementSum.Equal(onHand) {
		t.Fatalf("movements sum to %s but on hand is %s", movementSum, onHand)
	}
}

func TestIssueInvalid(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	acc := testdb.SeedAccount(t, pool)
	svc := stock.NewService(pool)

	err := svc.Receive(ctx, stock.ReceiveRequest{
		TenantID:   acc.TenantID,
		UserID:     acc.UserID,
		ProductID:  acc.ProductID,
		LocationID: acc.LocationID,
		Quantity:   decimal.NewFromInt(20),
	})
	if err != nil {
		t.Fatalf("receive: %v", err)
	}

	// A product of the same tenant that never received anything.
	var emptyProductID uuid.UUID

	err = pool.QueryRow(ctx,
		`INSERT INTO products (tenant_id, name, code, unit)
		 VALUES ($1, 'empty_product', 'empty_product_code', 'pcs') RETURNING id`,
		acc.TenantID).Scan(&emptyProductID)
	if err != nil {
		t.Fatalf("seed empty product: %v", err)
	}

	issue := func(quantity int64, reason stock.Reason, note string) stock.IssueRequest {
		return stock.IssueRequest{
			TenantID:   acc.TenantID,
			UserID:     acc.UserID,
			ProductID:  acc.ProductID,
			LocationID: acc.LocationID,
			Quantity:   decimal.NewFromInt(quantity),
			Reason:     reason,
			Note:       note,
		}
	}

	noStock := issue(1, stock.ReasonIssue, "sold")
	noStock.ProductID = emptyProductID

	tests := []struct {
		name    string
		req     stock.IssueRequest
		wantErr error
	}{
		{"zero quantity", issue(0, stock.ReasonIssue, "sold"), stock.ErrInvalidQuantity},
		{"negative quantity", issue(-5, stock.ReasonIssue, "sold"), stock.ErrInvalidQuantity},
		{"reason that is not an issue", issue(5, stock.ReasonReceive, "sold"), stock.ErrInvalidReason},
		{"more than on hand", issue(25, stock.ReasonIssue, "sold"), stock.ErrInsufficientStock},
		{"no stock at the position", noStock, stock.ErrInsufficientStock},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.Issue(ctx, tc.req)

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}

	// Nothing may have left: only the receipt remains, on hand untouched.
	var movementCount int

	err = pool.QueryRow(ctx,
		`SELECT count(*) FROM stock_movements WHERE tenant_id=$1`,
		acc.TenantID).Scan(&movementCount)
	if err != nil {
		t.Fatalf("count movements: %v", err)
	}

	if movementCount != 1 {
		t.Fatalf("expected 1 movement, got %d", movementCount)
	}

	onHand, err := svc.OnHand(ctx, stock.OnHandRequest{
		TenantID:   acc.TenantID,
		ProductID:  acc.ProductID,
		LocationID: acc.LocationID,
	})
	if err != nil {
		t.Fatalf("on hand: %v", err)
	}

	if !onHand.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("expected on hand 20, got %s", onHand)
	}
}

func TestMain(m *testing.M) {
	testdb.Main(m)
}
