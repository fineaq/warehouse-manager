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

	"github.com/jackc/pgx/v5/pgxpool"
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
