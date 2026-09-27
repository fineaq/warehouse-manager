package catalogue_test

import (
	"context"
	"errors"
	"testing"
	"warehouse-manager/internal/catalogue"
	"warehouse-manager/internal/testdb"

	"github.com/google/uuid"
)

func TestCreateProduct(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	acc := testdb.SeedAccount(t, pool)
	svc := catalogue.NewService(pool)

	productName := "test_product"
	productCode := "test_product code"
	unit := "test_unit"
	note := "test_note"

	err := svc.CreateProduct(ctx, catalogue.CreateProductRequest{
		TenantID: acc.TenantID,
		Name:     productName,
		Code:     productCode,
		Unit:     unit,
		Note:     note,
	})
	if err != nil {
		t.Fatalf("new product: %v", err)
	}

	var storedName, storedCode, storedUnit, storedNote string

	if err := pool.QueryRow(ctx,
		`SELECT name, code, unit, note
		FROM products
		WHERE tenant_id=$1 AND code=$2`,
		acc.TenantID, productCode,
	).Scan(&storedName, &storedCode, &storedUnit, &storedNote); err != nil {
		t.Fatalf("query product: %v", err)
	}

	if storedName != productName {
		t.Fatalf("expected name %q, got %q", productName, storedName)
	}

	if storedCode != productCode {
		t.Fatalf("expected code %q, got %q", productCode, storedCode)
	}

	if storedUnit != unit {
		t.Fatalf("expected unit %q, got %q", unit, storedUnit)
	}

	if storedNote != note {
		t.Fatalf("expected note %q, got %q", note, storedNote)
	}
}

func TestCreateProductInvalid(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	acc := testdb.SeedAccount(t, pool)
	svc := catalogue.NewService(pool)

	req := catalogue.CreateProductRequest{
		TenantID: acc.TenantID,
		Name:     "test_product",
		Code:     "duplicate_code",
		Unit:     "test_unit",
	}

	if err := svc.CreateProduct(ctx, req); err != nil {
		t.Fatalf("first new product: %v", err)
	}

	// A second product with the same code, even under a different name.
	req.Name = "another_product"

	err := svc.CreateProduct(ctx, req)
	if !errors.Is(err, catalogue.ErrDuplicateCode) {
		t.Fatalf("expected %v, got %v", catalogue.ErrDuplicateCode, err)
	}

	var count int

	err = pool.QueryRow(ctx,
		`SELECT count(*) FROM products WHERE tenant_id=$1 AND code=$2`,
		acc.TenantID, req.Code,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count products: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected 1 product, got %d", count)
	}
}

func TestListProduct(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	acc := testdb.SeedAccount(t, pool)
	svc := catalogue.NewService(pool)

	// SeedAccount already created one active product.
	err := svc.CreateProduct(ctx, catalogue.CreateProductRequest{
		TenantID: acc.TenantID,
		Name:     "active_product",
		Code:     "active_code",
		Unit:     "pcs",
	})
	if err != nil {
		t.Fatalf("new product: %v", err)
	}

	products, err := svc.ListProduct(ctx, acc.TenantID)
	if err != nil {
		t.Fatalf("list products: %v", err)
	}

	codes := make(map[string]bool, len(products))
	for _, p := range products {
		codes[p.Code] = true
	}

	if !codes["active_code"] {
		t.Fatalf("expected active_code in the list, got %v", codes)
	}

	// The seeded product plus the one created here.
	if len(products) != 2 {
		t.Fatalf("expected 2 products, got %d: %v", len(products), codes)
	}
}

func TestListProductInvalid(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	svc := catalogue.NewService(pool)

	mine := testdb.SeedAccount(t, pool)
	other := testdb.SeedAccount(t, pool)

	err := svc.CreateProduct(ctx, catalogue.CreateProductRequest{
		TenantID: other.TenantID,
		Name:     "other_tenant_product",
		Code:     "other_tenant_code",
		Unit:     "pcs",
	})
	if err != nil {
		t.Fatalf("new product for the other tenant: %v", err)
	}

	err = svc.CreateProduct(ctx, catalogue.CreateProductRequest{
		TenantID: mine.TenantID,
		Name:     "deactivated_product",
		Code:     "inactive_code",
		Unit:     "pcs",
	})
	if err != nil {
		t.Fatalf("new inactive product: %v", err)
	}

	_, err = pool.Exec(ctx,
		`UPDATE products SET active = false WHERE tenant_id=$1 AND code=$2`,
		mine.TenantID, "inactive_code")
	if err != nil {
		t.Fatalf("deactivate product: %v", err)
	}

	tests := []struct {
		name       string
		tenantID   uuid.UUID
		absentCode string
		wantEmpty  bool
	}{
		{
			name:       "inactive products are not listed",
			tenantID:   mine.TenantID,
			absentCode: "inactive_code",
		},
		{
			name:       "another tenant's products are not listed",
			tenantID:   mine.TenantID,
			absentCode: "other_tenant_code",
		},
		{
			name:      "unknown tenant returns an empty list",
			tenantID:  uuid.New(),
			wantEmpty: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			products, err := svc.ListProduct(ctx, tc.tenantID)
			if err != nil {
				t.Fatalf("list products: %v", err)
			}

			if tc.wantEmpty && len(products) != 0 {
				t.Fatalf("expected no products, got %d", len(products))
			}

			for _, p := range products {
				if tc.absentCode != "" && p.Code == tc.absentCode {
					t.Fatalf("%s must not be listed: %+v", tc.absentCode, p)
				}
			}
		})
	}
}

func TestUpdateProduct(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	acc := testdb.SeedAccount(t, pool)
	svc := catalogue.NewService(pool)

	err := svc.CreateProduct(ctx, catalogue.CreateProductRequest{
		TenantID: acc.TenantID,
		Name:     "product_update_code",
		Code:     "update_code",
		Unit:     "pcs",
	})
	if err != nil {
		t.Fatalf("new product: %v", err)
	}

	var productID uuid.UUID

	err = pool.QueryRow(ctx,
		`SELECT id FROM products WHERE tenant_id=$1 AND code=$2`,
		acc.TenantID, "update_code",
	).Scan(&productID)
	if err != nil {
		t.Fatalf("find product: %v", err)
	}

	err = svc.UpdateProduct(ctx, catalogue.UpdateProductRequest{
		TenantID: acc.TenantID,
		Id:       productID,
		Name:     "renamed_product",
		Code:     "update_code",
		Unit:     "kg",
		Note:     "renamed_note",
	})
	if err != nil {
		t.Fatalf("update product: %v", err)
	}

	var name, code, unit, note string

	err = pool.QueryRow(ctx,
		`SELECT name, code, unit, note FROM products WHERE tenant_id=$1 AND id=$2`,
		acc.TenantID, productID,
	).Scan(&name, &code, &unit, &note)
	if err != nil {
		t.Fatalf("query product: %v", err)
	}

	if name != "renamed_product" {
		t.Fatalf("expected name renamed_product, got %q", name)
	}

	if code != "update_code" {
		t.Fatalf("code must not change, got %q", code)
	}

	if unit != "kg" {
		t.Fatalf("expected unit kg, got %q", unit)
	}

	if note != "renamed_note" {
		t.Fatalf("expected note renamed_note, got %q", note)
	}
}

func TestUpdateProductInvalid(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	svc := catalogue.NewService(pool)

	mine := testdb.SeedAccount(t, pool)
	other := testdb.SeedAccount(t, pool)

	err := svc.CreateProduct(ctx, catalogue.CreateProductRequest{
		TenantID: other.TenantID,
		Name:     "other_tenant_product",
		Code:     "other_tenant_code",
		Unit:     "pcs",
	})
	if err != nil {
		t.Fatalf("new product for the other tenant: %v", err)
	}

	var otherProductID uuid.UUID

	err = pool.QueryRow(ctx,
		`SELECT id FROM products WHERE tenant_id=$1 AND code=$2`,
		other.TenantID, "other_tenant_code",
	).Scan(&otherProductID)
	if err != nil {
		t.Fatalf("find the other tenant's product: %v", err)
	}

	for _, code := range []string{"mine_code_a", "mine_code_b"} {
		err = svc.CreateProduct(ctx, catalogue.CreateProductRequest{
			TenantID: mine.TenantID,
			Name:     "product_" + code,
			Code:     code,
			Unit:     "pcs",
		})
		if err != nil {
			t.Fatalf("new product %s: %v", code, err)
		}
	}

	var myProductID uuid.UUID

	err = pool.QueryRow(ctx,
		`SELECT id FROM products WHERE tenant_id=$1 AND code=$2`,
		mine.TenantID, "mine_code_a",
	).Scan(&myProductID)
	if err != nil {
		t.Fatalf("find my product: %v", err)
	}

	tests := []struct {
		name    string
		req     catalogue.UpdateProductRequest
		wantErr error
	}{
		{
			name: "unknown product",
			req: catalogue.UpdateProductRequest{
				TenantID: mine.TenantID,
				Id:       uuid.New(),
				Name:     "renamed_product",
			},
			wantErr: catalogue.ErrProductNotFound,
		},
		{
			name: "another tenant's product",
			req: catalogue.UpdateProductRequest{
				TenantID: mine.TenantID,
				Id:       otherProductID,
				Name:     "renamed_product",
			},
			wantErr: catalogue.ErrProductNotFound,
		},
		{
			name: "code already taken",
			req: catalogue.UpdateProductRequest{
				TenantID: mine.TenantID,
				Id:       myProductID,
				Name:     "renamed_product",
				Code:     "mine_code_b",
				Unit:     "pcs",
			},
			wantErr: catalogue.ErrDuplicateCode,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.UpdateProduct(ctx, tc.req)

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}

	// The rejected rename must not have changed anything.
	var myCode string

	err = pool.QueryRow(ctx,
		`SELECT code FROM products WHERE tenant_id=$1 AND id=$2`,
		mine.TenantID, myProductID,
	).Scan(&myCode)
	if err != nil {
		t.Fatalf("query my product: %v", err)
	}

	if myCode != "mine_code_a" {
		t.Fatalf("code must not change, got %q", myCode)
	}

	var name string

	err = pool.QueryRow(ctx,
		`SELECT name FROM products WHERE tenant_id=$1 AND id=$2`,
		other.TenantID, otherProductID,
	).Scan(&name)
	if err != nil {
		t.Fatalf("query the other tenant's product: %v", err)
	}

	if name != "other_tenant_product" {
		t.Fatalf("the other tenant's product was modified: name is now %q", name)
	}
}

func TestCreateLocation(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	acc := testdb.SeedAccount(t, pool)
	svc := catalogue.NewService(pool)

	locationName := "test_location"
	locationCode := "test_location code"
	address := "test_address"
	note := "test_note"

	err := svc.CreateLocation(ctx, catalogue.CreateLocationRequest{
		TenantID: acc.TenantID,
		Name:     locationName,
		Code:     locationCode,
		Address:  address,
		Note:     note,
	})
	if err != nil {
		t.Fatalf("new location: %v", err)
	}

	var storedName, storedCode, storedAddress, storedNote string

	if err := pool.QueryRow(ctx,
		`SELECT name, code, address, note
		FROM locations
		WHERE tenant_id=$1 AND code=$2`,
		acc.TenantID, locationCode,
	).Scan(&storedName, &storedCode, &storedAddress, &storedNote); err != nil {
		t.Fatalf("query location: %v", err)
	}

	if storedName != locationName {
		t.Fatalf("expected name %q, got %q", locationName, storedName)
	}

	if storedCode != locationCode {
		t.Fatalf("expected code %q, got %q", locationCode, storedCode)
	}

	if storedAddress != address {
		t.Fatalf("expected address %q, got %q", address, storedAddress)
	}

	if storedNote != note {
		t.Fatalf("expected note %q, got %q", note, storedNote)
	}
}

func TestCreateLocationInvalid(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	acc := testdb.SeedAccount(t, pool)
	svc := catalogue.NewService(pool)

	req := catalogue.CreateLocationRequest{
		TenantID: acc.TenantID,
		Name:     "test_location",
		Code:     "duplicate_code",
		Address:  "test_address",
	}

	if err := svc.CreateLocation(ctx, req); err != nil {
		t.Fatalf("first new location: %v", err)
	}

	// A second location with the same code, even under a different name.
	req.Name = "another_location"

	err := svc.CreateLocation(ctx, req)
	if !errors.Is(err, catalogue.ErrDuplicateCode) {
		t.Fatalf("expected %v, got %v", catalogue.ErrDuplicateCode, err)
	}

	var count int

	err = pool.QueryRow(ctx,
		`SELECT count(*) FROM locations WHERE tenant_id=$1 AND code=$2`,
		acc.TenantID, req.Code,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count locations: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected 1 location, got %d", count)
	}
}

func TestUpdateLocation(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	acc := testdb.SeedAccount(t, pool)
	svc := catalogue.NewService(pool)

	err := svc.CreateLocation(ctx, catalogue.CreateLocationRequest{
		TenantID: acc.TenantID,
		Name:     "location_update_code",
		Code:     "update_code",
		Address:  "old_address",
	})
	if err != nil {
		t.Fatalf("new location: %v", err)
	}

	var locationID uuid.UUID

	err = pool.QueryRow(ctx,
		`SELECT id FROM locations WHERE tenant_id=$1 AND code=$2`,
		acc.TenantID, "update_code",
	).Scan(&locationID)
	if err != nil {
		t.Fatalf("find location: %v", err)
	}

	err = svc.UpdateLocation(ctx, catalogue.UpdateLocationRequest{
		TenantID: acc.TenantID,
		Id:       locationID,
		Name:     "renamed_location",
		Code:     "update_code",
		Address:  "new_address",
		Note:     "new_note",
	})
	if err != nil {
		t.Fatalf("update location: %v", err)
	}

	var name, code, address, note string

	err = pool.QueryRow(ctx,
		`SELECT name, code, address, note FROM locations WHERE tenant_id=$1 AND id=$2`,
		acc.TenantID, locationID,
	).Scan(&name, &code, &address, &note)
	if err != nil {
		t.Fatalf("query location: %v", err)
	}

	if name != "renamed_location" {
		t.Fatalf("expected name renamed_location, got %q", name)
	}

	if code != "update_code" {
		t.Fatalf("code must not change, got %q", code)
	}

	if address != "new_address" {
		t.Fatalf("expected address new_address, got %q", address)
	}

	if note != "new_note" {
		t.Fatalf("expected note new_note, got %q", note)
	}
}

func TestUpdateLocationInvalid(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	svc := catalogue.NewService(pool)

	mine := testdb.SeedAccount(t, pool)
	other := testdb.SeedAccount(t, pool)

	err := svc.CreateLocation(ctx, catalogue.CreateLocationRequest{
		TenantID: other.TenantID,
		Name:     "other_tenant_location",
		Code:     "other_tenant_code",
	})
	if err != nil {
		t.Fatalf("new location for the other tenant: %v", err)
	}

	var otherLocationID uuid.UUID

	err = pool.QueryRow(ctx,
		`SELECT id FROM locations WHERE tenant_id=$1 AND code=$2`,
		other.TenantID, "other_tenant_code",
	).Scan(&otherLocationID)
	if err != nil {
		t.Fatalf("find the other tenant's location: %v", err)
	}

	for _, code := range []string{"mine_code_a", "mine_code_b"} {
		err = svc.CreateLocation(ctx, catalogue.CreateLocationRequest{
			TenantID: mine.TenantID,
			Name:     "location_" + code,
			Code:     code,
		})
		if err != nil {
			t.Fatalf("new location %s: %v", code, err)
		}
	}

	var myLocationID uuid.UUID

	err = pool.QueryRow(ctx,
		`SELECT id FROM locations WHERE tenant_id=$1 AND code=$2`,
		mine.TenantID, "mine_code_a",
	).Scan(&myLocationID)
	if err != nil {
		t.Fatalf("find my location: %v", err)
	}

	tests := []struct {
		name    string
		req     catalogue.UpdateLocationRequest
		wantErr error
	}{
		{
			name: "unknown location",
			req: catalogue.UpdateLocationRequest{
				TenantID: mine.TenantID,
				Id:       uuid.New(),
				Name:     "renamed_location",
				Code:     "unknown_code",
			},
			wantErr: catalogue.ErrLocationNotFound,
		},
		{
			name: "another tenant's location",
			req: catalogue.UpdateLocationRequest{
				TenantID: mine.TenantID,
				Id:       otherLocationID,
				Name:     "renamed_location",
				Code:     "other_tenant_code",
			},
			wantErr: catalogue.ErrLocationNotFound,
		},
		{
			name: "code already taken",
			req: catalogue.UpdateLocationRequest{
				TenantID: mine.TenantID,
				Id:       myLocationID,
				Name:     "renamed_location",
				Code:     "mine_code_b",
			},
			wantErr: catalogue.ErrDuplicateCode,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.UpdateLocation(ctx, tc.req)

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}

	// The rejected rename must not have changed anything.
	var myName, myCode string

	err = pool.QueryRow(ctx,
		`SELECT name, code FROM locations WHERE tenant_id=$1 AND id=$2`,
		mine.TenantID, myLocationID,
	).Scan(&myName, &myCode)
	if err != nil {
		t.Fatalf("query my location: %v", err)
	}

	if myName != "location_mine_code_a" || myCode != "mine_code_a" {
		t.Fatalf("my location must not change, got name %q code %q", myName, myCode)
	}

	var name string

	err = pool.QueryRow(ctx,
		`SELECT name FROM locations WHERE tenant_id=$1 AND id=$2`,
		other.TenantID, otherLocationID,
	).Scan(&name)
	if err != nil {
		t.Fatalf("query the other tenant's location: %v", err)
	}

	if name != "other_tenant_location" {
		t.Fatalf("the other tenant's location was modified: name is now %q", name)
	}
}

func TestListLocation(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	acc := testdb.SeedAccount(t, pool)
	svc := catalogue.NewService(pool)

	// SeedAccount already created one active location.
	err := svc.CreateLocation(ctx, catalogue.CreateLocationRequest{
		TenantID: acc.TenantID,
		Name:     "active_location",
		Code:     "active_code",
		Address:  "test_address",
	})
	if err != nil {
		t.Fatalf("new location: %v", err)
	}

	locations, err := svc.ListLocation(ctx, acc.TenantID)
	if err != nil {
		t.Fatalf("list locations: %v", err)
	}

	codes := make(map[string]bool, len(locations))
	for _, l := range locations {
		codes[l.Code] = true
	}

	if !codes["active_code"] {
		t.Fatalf("expected active_code in the list, got %v", codes)
	}

	// The seeded location plus the one created here.
	if len(locations) != 2 {
		t.Fatalf("expected 2 locations, got %d: %v", len(locations), codes)
	}
}

func TestListLocationInvalid(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	svc := catalogue.NewService(pool)

	mine := testdb.SeedAccount(t, pool)
	other := testdb.SeedAccount(t, pool)

	err := svc.CreateLocation(ctx, catalogue.CreateLocationRequest{
		TenantID: other.TenantID,
		Name:     "other_tenant_location",
		Code:     "other_tenant_code",
	})
	if err != nil {
		t.Fatalf("new location for the other tenant: %v", err)
	}

	err = svc.CreateLocation(ctx, catalogue.CreateLocationRequest{
		TenantID: mine.TenantID,
		Name:     "deactivated_location",
		Code:     "inactive_code",
	})
	if err != nil {
		t.Fatalf("new inactive location: %v", err)
	}

	_, err = pool.Exec(ctx,
		`UPDATE locations SET active = false WHERE tenant_id=$1 AND code=$2`,
		mine.TenantID, "inactive_code")
	if err != nil {
		t.Fatalf("deactivate location: %v", err)
	}

	tests := []struct {
		name       string
		tenantID   uuid.UUID
		absentCode string
		wantEmpty  bool
	}{
		{
			name:       "inactive locations are not listed",
			tenantID:   mine.TenantID,
			absentCode: "inactive_code",
		},
		{
			name:       "another tenant's locations are not listed",
			tenantID:   mine.TenantID,
			absentCode: "other_tenant_code",
		},
		{
			name:      "unknown tenant returns an empty list",
			tenantID:  uuid.New(),
			wantEmpty: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			locations, err := svc.ListLocation(ctx, tc.tenantID)
			if err != nil {
				t.Fatalf("list locations: %v", err)
			}

			if tc.wantEmpty && len(locations) != 0 {
				t.Fatalf("expected no locations, got %d", len(locations))
			}

			// Only the location SeedAccount created, whose code is the account email.
			if !tc.wantEmpty && (len(locations) != 1 || locations[0].Code != mine.Email) {
				t.Fatalf("expected only the seeded location %s, got %+v", mine.Email, locations)
			}

			for _, l := range locations {
				if tc.absentCode != "" && l.Code == tc.absentCode {
					t.Fatalf("%s must not be listed: %+v", tc.absentCode, l)
				}
			}
		})
	}
}

func TestMain(m *testing.M) {
	testdb.Main(m)
}
