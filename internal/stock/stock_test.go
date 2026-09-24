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
				t.Fatalf("receive : %v", err)
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
				t.Fatalf("expected on_hand %d got %s", tc.wantOnHand, onhand)
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

	tests := []struct {
		name    string
		req     stock.ReceiveRequest
		wantErr error
	}{
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

			if err == nil {
				t.Fatalf("expected error, got nil")
			}

			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected error %v, got %v", tc.wantErr, err)
			}
		})
	}

	var movementCount, balanceCount int

	err := pool.QueryRow(ctx,
		`SELECT count(*) FROM stock_movements WHERE tenant_id=$1`,
		acc.TenantID).Scan(&movementCount)
	if err != nil {
		t.Fatalf("query movements: %v", err)
	}

	if movementCount != 0 {
		t.Fatalf("expected 0 movements, got %d", movementCount)
	}

	err = pool.QueryRow(ctx,
		`SELECT count(*) FROM stock_balances WHERE tenant_id=$1`,
		acc.TenantID).Scan(&balanceCount)
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

func TestMain(m *testing.M) {
	testdb.Main(m)
}
