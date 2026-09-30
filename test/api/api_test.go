// Package api drives the assembled application over HTTP: router, middleware,
// auth, stock and catalogue together.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"warehouse-manager/internal/auth"
	"warehouse-manager/internal/catalogue"
	"warehouse-manager/internal/router"
	"warehouse-manager/internal/stock"
	"warehouse-manager/internal/testdb"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

const testSecret = "test-only-jwt-secret-at-least-32-bytes-long"

// newServer builds the router main.go builds, so requests pass through the
// real authentication middleware.
func newServer(t *testing.T, pool *pgxpool.Pool) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)

	authSvc := auth.NewService(pool, []byte(testSecret))

	return router.NewRouter(authSvc,
		[]router.RouteRegistrar{auth.NewHandler(authSvc)},
		[]router.RouteRegistrar{
			stock.NewHandler(stock.NewService(pool)),
			catalogue.NewHandler(catalogue.NewService(pool)),
		},
	)
}

func do(t *testing.T, engine *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	return w
}

func login(t *testing.T, engine *gin.Engine, email string) string {
	t.Helper()

	w := do(t, engine, http.MethodPost, "/v1/auth/login",
		fmt.Sprintf(`{"email":%q,"password":%q}`, email, testdb.Password), "")

	if w.Code != http.StatusOK {
		t.Fatalf("login: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Token string `json:"token"`
	}

	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}

	if resp.Token == "" {
		t.Fatalf("login returned an empty token: %s", w.Body.String())
	}

	return resp.Token
}

func TestStockEndpointsRejectRequestWithoutToken(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	acc := testdb.SeedAccount(t, pool)
	engine := newServer(t, pool)

	receipt := fmt.Sprintf(`{"location_id":%q,"product_id":%q,"quantity":20}`,
		acc.LocationID, acc.ProductID)

	onHand := fmt.Sprintf("/v1/on-hand?product_id=%s&location_id=%s",
		acc.ProductID, acc.LocationID)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		token  string
	}{
		{"receipt without a token", http.MethodPost, "/v1/receipts", receipt, ""},
		{"receipt with a token that is not a JWT", http.MethodPost, "/v1/receipts", receipt, "not-a-token"},
		{"issue without a token", http.MethodPost, "/v1/issue", receipt, ""},
		{"on hand without a token", http.MethodGet, onHand, "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := do(t, engine, tc.method, tc.path, tc.body, tc.token)

			if w.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
			}
		})
	}

	var movementCount int

	err := pool.QueryRow(ctx,
		`SELECT count(*) FROM stock_movements WHERE tenant_id=$1`,
		acc.TenantID).Scan(&movementCount)
	if err != nil {
		t.Fatalf("query movements: %v", err)
	}

	if movementCount != 0 {
		t.Fatalf("expected 0 movements, got %d", movementCount)
	}
}

func TestReceiveStoresIdentityFromToken(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	acc := testdb.SeedAccount(t, pool)
	engine := newServer(t, pool)
	token := login(t, engine, acc.Email)

	otherTenantID := uuid.New()
	otherUserID := uuid.New()

	body := fmt.Sprintf(
		`{"location_id":%q,"product_id":%q,"quantity":20,"tenant_id":%q,"user_id":%q}`,
		acc.LocationID, acc.ProductID, otherTenantID, otherUserID)

	w := do(t, engine, http.MethodPost, "/v1/receipts", body, token)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var storedTenantID, storedUserID uuid.UUID

	err := pool.QueryRow(ctx,
		`SELECT tenant_id, user_id FROM stock_movements WHERE tenant_id=$1`,
		acc.TenantID).Scan(&storedTenantID, &storedUserID)
	if err != nil {
		t.Fatalf("query movement: %v", err)
	}

	if storedUserID != acc.UserID {
		t.Fatalf("expected movement user %s, got %s", acc.UserID, storedUserID)
	}

	if storedTenantID != acc.TenantID {
		t.Fatalf("expected movement tenant %s, got %s", acc.TenantID, storedTenantID)
	}

	var forged int

	err = pool.QueryRow(ctx,
		`SELECT count(*) FROM stock_movements WHERE tenant_id=$1 OR user_id=$2`,
		otherTenantID, otherUserID).Scan(&forged)
	if err != nil {
		t.Fatalf("query forged movements: %v", err)
	}

	if forged != 0 {
		t.Fatalf("expected 0 movements for the identity in the body, got %d", forged)
	}
}

func TestReceiveThenReadOnHand(t *testing.T) {
	pool := testdb.New(t)
	acc := testdb.SeedAccount(t, pool)
	engine := newServer(t, pool)
	token := login(t, engine, acc.Email)

	receipt := fmt.Sprintf(`{"location_id":%q,"product_id":%q,"quantity":20}`,
		acc.LocationID, acc.ProductID)

	w := do(t, engine, http.MethodPost, "/v1/receipts", receipt, token)

	if w.Code != http.StatusCreated {
		t.Fatalf("receipt: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	onHand := fmt.Sprintf("/v1/on-hand?product_id=%s&location_id=%s",
		acc.ProductID, acc.LocationID)

	w = do(t, engine, http.MethodGet, onHand, "", token)

	if w.Code != http.StatusOK {
		t.Fatalf("on hand: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Quantity decimal.Decimal `json:"quantity"`
	}

	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode on hand response: %v", err)
	}

	if !resp.Quantity.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("expected quantity 20, got %s: %s", resp.Quantity, w.Body.String())
	}
}

func TestReceiveRejectsInvalidReceipt(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	acc := testdb.SeedAccount(t, pool)
	engine := newServer(t, pool)
	token := login(t, engine, acc.Email)

	receipt := func(locationID, productID uuid.UUID, quantity string) string {
		return fmt.Sprintf(`{"location_id":%q,"product_id":%q,"quantity":%s}`,
			locationID, productID, quantity)
	}

	var inactiveProductID, inactiveLocationID uuid.UUID

	err := pool.QueryRow(ctx,
		`INSERT INTO products (tenant_id, name, code, unit, active)
		 VALUES ($1, 'inactive_product', 'inactive_product_code', 'pcs', false) RETURNING id`,
		acc.TenantID).Scan(&inactiveProductID)
	if err != nil {
		t.Fatalf("seed inactive product: %v", err)
	}

	err = pool.QueryRow(ctx,
		`INSERT INTO locations (tenant_id, name, code, active)
		 VALUES ($1, 'inactive_location', 'inactive_location_code', false) RETURNING id`,
		acc.TenantID).Scan(&inactiveLocationID)
	if err != nil {
		t.Fatalf("seed inactive location: %v", err)
	}

	tests := []struct {
		name string
		body string
	}{
		{"zero quantity", receipt(acc.LocationID, acc.ProductID, "0")},
		{"negative quantity", receipt(acc.LocationID, acc.ProductID, "-5")},
		{"unknown product", receipt(acc.LocationID, uuid.New(), "20")},
		{"unknown location", receipt(uuid.New(), acc.ProductID, "20")},
		{"inactive product", receipt(acc.LocationID, inactiveProductID, "20")},
		{"inactive location", receipt(inactiveLocationID, acc.ProductID, "20")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := do(t, engine, http.MethodPost, "/v1/receipts", tc.body, token)

			if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
			}
		})
	}

	var movementCount int

	err = pool.QueryRow(ctx,
		`SELECT count(*) FROM stock_movements WHERE tenant_id=$1`,
		acc.TenantID).Scan(&movementCount)
	if err != nil {
		t.Fatalf("query movements: %v", err)
	}

	if movementCount != 0 {
		t.Fatalf("expected 0 movements, got %d", movementCount)
	}
}

func TestCreateProductThenList(t *testing.T) {
	pool := testdb.New(t)
	acc := testdb.SeedAccount(t, pool)
	engine := newServer(t, pool)
	token := login(t, engine, acc.Email)

	body := `{"name":"widget","code":"WID-1","unit":"pcs","note":"a widget"}`

	w := do(t, engine, http.MethodPost, "/v1/products", body, token)

	if w.Code != http.StatusCreated {
		t.Fatalf("create product: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, engine, http.MethodGet, "/v1/products", "", token)

	if w.Code != http.StatusOK {
		t.Fatalf("list products: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Products []struct {
			Name string `json:"name"`
			Code string `json:"code"`
			Unit string `json:"unit"`
			Note string `json:"note"`
		} `json:"products"`
	}

	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode products: %v", err)
	}

	var found bool

	for _, p := range resp.Products {
		if p.Code == "WID-1" {
			found = true

			if p.Name != "widget" || p.Unit != "pcs" || p.Note != "a widget" {
				t.Fatalf("stored product mismatch: %+v", p)
			}
		}
	}

	if !found {
		t.Fatalf("created product not in list: %s", w.Body.String())
	}
}

func TestCreateProductRejectsDuplicateCode(t *testing.T) {
	pool := testdb.New(t)
	acc := testdb.SeedAccount(t, pool)
	engine := newServer(t, pool)
	token := login(t, engine, acc.Email)

	body := `{"name":"first","code":"DUP","unit":"pcs","note":"first note"}`

	w := do(t, engine, http.MethodPost, "/v1/products", body, token)

	if w.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	body = `{"name":"second","code":"DUP","unit":"pcs","note":"second note"}`

	w = do(t, engine, http.MethodPost, "/v1/products", body, token)

	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate code: expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateProduct(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	acc := testdb.SeedAccount(t, pool)
	engine := newServer(t, pool)
	token := login(t, engine, acc.Email)

	body := `{"name":"before","code":"UPD","unit":"pcs","note":"old note"}`

	w := do(t, engine, http.MethodPost, "/v1/products", body, token)

	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var id uuid.UUID

	err := pool.QueryRow(ctx,
		`SELECT id FROM products WHERE tenant_id=$1 AND code=$2`,
		acc.TenantID, "UPD").Scan(&id)
	if err != nil {
		t.Fatalf("query product id: %v", err)
	}

	body = `{"name":"after","code":"UPD","unit":"kg","note":"new note"}`

	w = do(t, engine, http.MethodPut, "/v1/products/"+id.String(), body, token)

	if w.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var name, unit, note string

	err = pool.QueryRow(ctx,
		`SELECT name, unit, note FROM products WHERE tenant_id=$1 AND id=$2`,
		acc.TenantID, id).Scan(&name, &unit, &note)
	if err != nil {
		t.Fatalf("query product: %v", err)
	}

	if name != "after" || unit != "kg" || note != "new note" {
		t.Fatalf("product not updated: name=%q unit=%q note=%q", name, unit, note)
	}
}

func TestUpdateProductInvalid(t *testing.T) {
	pool := testdb.New(t)
	acc := testdb.SeedAccount(t, pool)
	engine := newServer(t, pool)
	token := login(t, engine, acc.Email)

	body := `{"name":"x","code":"x","unit":"pcs","note":"a note"}`

	tests := []struct {
		name     string
		id       string
		wantCode int
	}{
		{"unknown id", uuid.New().String(), http.StatusNotFound},
		{"malformed id", "not-a-uuid", http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := do(t, engine, http.MethodPut, "/v1/products/"+tc.id, body, token)

			if w.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d: %s", tc.wantCode, w.Code, w.Body.String())
			}
		})
	}
}

func TestCreateLocationThenList(t *testing.T) {
	pool := testdb.New(t)
	acc := testdb.SeedAccount(t, pool)
	engine := newServer(t, pool)
	token := login(t, engine, acc.Email)

	body := `{"name":"dock","code":"LOC-1","address":"12 Main St","note":"loading dock"}`

	w := do(t, engine, http.MethodPost, "/v1/locations", body, token)

	if w.Code != http.StatusCreated {
		t.Fatalf("create location: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, engine, http.MethodGet, "/v1/locations", "", token)

	if w.Code != http.StatusOK {
		t.Fatalf("list locations: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Locations []struct {
			Code    string `json:"code"`
			Address string `json:"address"`
		} `json:"locations"`
	}

	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode locations: %v", err)
	}

	var found bool

	for _, l := range resp.Locations {
		if l.Code == "LOC-1" {
			found = true

			if l.Address != "12 Main St" {
				t.Fatalf("stored address mismatch: %q", l.Address)
			}
		}
	}

	if !found {
		t.Fatalf("created location not in list: %s", w.Body.String())
	}
}

func TestCatalogueEndpointsRejectRequestWithoutToken(t *testing.T) {
	pool := testdb.New(t)
	acc := testdb.SeedAccount(t, pool)
	engine := newServer(t, pool)

	product := `{"name":"n","code":"c","unit":"pcs","note":""}`

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"list products", http.MethodGet, "/v1/products", ""},
		{"create product", http.MethodPost, "/v1/products", product},
		{"update product", http.MethodPut, "/v1/products/" + acc.ProductID.String(), product},
		{"list locations", http.MethodGet, "/v1/locations", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := do(t, engine, tc.method, tc.path, tc.body, "")

			if w.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestIssueThenReadOnHand(t *testing.T) {
	pool := testdb.New(t)

	acc := testdb.SeedAccount(t, pool)
	engine := newServer(t, pool)
	token := login(t, engine, acc.Email)

	receipt := fmt.Sprintf(`{"location_id":%q,"product_id":%q,"quantity":20}`,
		acc.LocationID, acc.ProductID)

	w := do(t, engine, http.MethodPost, "/v1/receipts", receipt, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("receipt: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	issue := fmt.Sprintf(`{"location_id":%q,"product_id":%q,"quantity":5,"reason":"issue","note":"sold"}`,
		acc.LocationID, acc.ProductID)

	w = do(t, engine, http.MethodPost, "/v1/issue", issue, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("issue: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	onHand := fmt.Sprintf("/v1/on-hand?product_id=%s&location_id=%s",
		acc.ProductID, acc.LocationID)

	w = do(t, engine, http.MethodGet, onHand, "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("on hand: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Quantity decimal.Decimal `json:"quantity"`
	}

	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode on hand response: %v", err)
	}

	if !resp.Quantity.Equal(decimal.NewFromInt(15)) {
		t.Fatalf("expected quantity 15, got %s: %s", resp.Quantity, w.Body.String())
	}
}

func TestIssueRejectsInvalidIssue(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	acc := testdb.SeedAccount(t, pool)
	engine := newServer(t, pool)
	token := login(t, engine, acc.Email)

	receipt := fmt.Sprintf(`{"location_id":%q,"product_id":%q,"quantity":20}`,
		acc.LocationID, acc.ProductID)

	w := do(t, engine, http.MethodPost, "/v1/receipts", receipt, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("receipt: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	issue := func(productID uuid.UUID, quantity, reason string) string {
		return fmt.Sprintf(`{"location_id":%q,"product_id":%q,"quantity":%s,"reason":%q,"note":"sold"}`,
			acc.LocationID, productID, quantity, reason)
	}

	tests := []struct {
		name string
		body string
		want int
	}{
		{"zero quantity", issue(acc.ProductID, "0", "issue"), http.StatusUnprocessableEntity},
		{"negative quantity", issue(acc.ProductID, "-5", "issue"), http.StatusUnprocessableEntity},
		{"reason that is not an issue", issue(acc.ProductID, "5", "receive"), http.StatusUnprocessableEntity},
		{"unknown product", issue(uuid.New(), "5", "issue"), http.StatusUnprocessableEntity},
		{"more than on hand", issue(acc.ProductID, "25", "issue"), http.StatusConflict},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := do(t, engine, http.MethodPost, "/v1/issue", tc.body, token)

			if w.Code != tc.want {
				t.Fatalf("expected %d, got %d: %s", tc.want, w.Code, w.Body.String())
			}
		})
	}

	// Only the receipt may have been stored, and it must be untouched.
	var movementCount int

	err := pool.QueryRow(ctx,
		`SELECT count(*) FROM stock_movements WHERE tenant_id=$1`,
		acc.TenantID).Scan(&movementCount)
	if err != nil {
		t.Fatalf("count movements: %v", err)
	}

	if movementCount != 1 {
		t.Fatalf("expected 1 movement, got %d", movementCount)
	}

	var onHand decimal.Decimal

	err = pool.QueryRow(ctx,
		`SELECT on_hand FROM stock_balances
		 WHERE tenant_id=$1 AND product_id=$2 AND location_id=$3`,
		acc.TenantID, acc.ProductID, acc.LocationID).Scan(&onHand)
	if err != nil {
		t.Fatalf("query balance: %v", err)
	}

	if !onHand.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("expected on hand 20, got %s", onHand)
	}
}

func TestMain(m *testing.M) {
	testdb.Main(m)
}
