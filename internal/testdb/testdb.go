package testdb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"testing"
	"warehouse-manager/internal/db"

	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

var (
	once      sync.Once
	pool      *pgxpool.Pool
	container *postgres.PostgresContainer
	startErr  error
)

func New(t *testing.T) *pgxpool.Pool {
	t.Helper()

	once.Do(start)

	if startErr != nil {
		t.Fatalf("start test database : %v", startErr)
	}

	return pool
}

const Password = "test-password"

type Account struct {
	Email      string
	TenantID   uuid.UUID
	UserID     uuid.UUID
	LocationID uuid.UUID
	ProductID  uuid.UUID
}

// SeedAccount creates a tenant, a user who can sign in, a location and a
// product.
func SeedAccount(t *testing.T, pool *pgxpool.Pool) Account {
	t.Helper()

	ctx := context.Background()
	acc := Account{Email: fmt.Sprintf("seed-%s@test.co", uuid.New())}

	hash, err := bcrypt.GenerateFromPassword([]byte(Password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	err = pool.QueryRow(ctx,
		`INSERT INTO tenants (name, email) VALUES ('test_tenant', $1) RETURNING id`,
		acc.Email).Scan(&acc.TenantID)
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}

	err = pool.QueryRow(ctx,
		`INSERT INTO users (tenant_id, name, email, password_hash)
		 VALUES ($1, 'test_user', $2, $3) RETURNING id`,
		acc.TenantID, acc.Email, string(hash)).Scan(&acc.UserID)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	err = pool.QueryRow(ctx,
		`INSERT INTO locations (tenant_id, name, code)
		 VALUES ($1, 'test_location', $2) RETURNING id`,
		acc.TenantID, acc.Email).Scan(&acc.LocationID)
	if err != nil {
		t.Fatalf("seed location: %v", err)
	}

	err = pool.QueryRow(ctx,
		`INSERT INTO products (tenant_id, name, code, unit)
		 VALUES ($1, 'test_product', $2, 'pcs') RETURNING id`,
		acc.TenantID, acc.Email).Scan(&acc.ProductID)
	if err != nil {
		t.Fatalf("seed product: %v", err)
	}

	return acc
}

func Main(m *testing.M) {
	code := m.Run()

	if pool != nil {
		pool.Close()
	}

	if container != nil {
		_ = container.Terminate(context.Background())
	}

	os.Exit(code)
}

func start() {
	ctx := context.Background()

	var err error

	container, err = postgres.Run(ctx, "postgres:17",
		postgres.WithDatabase("inventory_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.BasicWaitStrategies(),
	)

	if err != nil {
		startErr = fmt.Errorf("start container: %w", err)
		return
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		startErr = fmt.Errorf("connection string: %w", err)
		return
	}

	pool, err = db.NewPool(ctx, dsn)
	if err != nil {
		startErr = fmt.Errorf("new pool: %w", err)
		return
	}

	startErr = migrate(ctx, pool)

}

func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	_, thisFile, _, _ := runtime.Caller(0)
	migrationsDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")

	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.up.sql"))
	if err != nil {
		return fmt.Errorf("migrations : %w", err)
	}
	sort.Strings(files)

	for _, f := range files {
		sqlBytes, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read %s : %w", f, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply %s : %w", f, err)
		}
	}

	return nil
}
