package testdb

import (
	"context"
	"testing"
)

func TestNew(t *testing.T) {
	pool := New(t)

	var count int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'`,
	).Scan(&count)
	if err != nil {
		t.Fatalf("query tables: %v", err)
	}

	if count == 0 {
		t.Fatalf("expected tables after migration, found none")
	}
	t.Logf("found %d tables", count)
}
