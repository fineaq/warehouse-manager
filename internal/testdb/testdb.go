package testdb

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"warehouse-manager/internal/db"

	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/jackc/pgx/v5/pgxpool"
)

func New(t *testing.T) *pgxpool.Pool {
	t.Helper()

	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx, "postgres:17",
		postgres.WithDatabase("inventory_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.BasicWaitStrategies(),
	)

	testcontainers.CleanupContainer(t, pgContainer)
	if err != nil {
		t.Fatalf("start container: %v", err)
	}

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("start container : %v", err)
	}

	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("start container : %v", err)
	}

	t.Cleanup(func() { pool.Close() })

	//migration
	_, thisFile, _, _ := runtime.Caller(0)
	migrationsDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")

	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.up.sql"))
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	sort.Strings(files)

	for _, f := range files {
		sqlBytes, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("apply %s: %v", f, err)
		}
	}

	return pool
}
