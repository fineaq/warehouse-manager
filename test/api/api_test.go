// Package api drives the assembled application over HTTP: router, middleware,
// auth and stock together.
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
		[]router.RouteRegistrar{stock.NewHandler(stock.NewService(pool))},
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

	onHand := fmt.Sprintf("/v1/stock/on-hand?product_id=%s&location_id=%s",
		acc.ProductID, acc.LocationID)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		token  string
	}{
		{"receipt without a token", http.MethodPost, "/v1/stock/receipts", receipt, ""},
		{"receipt with a token that is not a JWT", http.MethodPost, "/v1/stock/receipts", receipt, "not-a-token"},
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

	w := do(t, engine, http.MethodPost, "/v1/stock/receipts", body, token)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
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

	w := do(t, engine, http.MethodPost, "/v1/stock/receipts", receipt, token)

	if w.Code != http.StatusOK {
		t.Fatalf("receipt: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	onHand := fmt.Sprintf("/v1/stock/on-hand?product_id=%s&location_id=%s",
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

	tests := []struct {
		name string
		body string
	}{
		{"zero quantity", receipt(acc.LocationID, acc.ProductID, "0")},
		{"negative quantity", receipt(acc.LocationID, acc.ProductID, "-5")},
		{"unknown product", receipt(acc.LocationID, uuid.New(), "20")},
		{"unknown location", receipt(uuid.New(), acc.ProductID, "20")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := do(t, engine, http.MethodPost, "/v1/stock/receipts", tc.body, token)

			if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
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

func TestMain(m *testing.M) {
	testdb.Main(m)
}
