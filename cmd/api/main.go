package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"warehouse-manager/internal/auth"
	"warehouse-manager/internal/db"
	"warehouse-manager/internal/router"
	"warehouse-manager/internal/stock"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		slog.Error("DATABASE_URL not set")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		slog.Error("connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	slog.Info("database connected")

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		slog.Error("JWT_SECRET not set")
		os.Exit(1)
	}

	if len(secret) < 32 {
		slog.Error("JWT_SECRET invalid length")
		os.Exit(1)
	}

	authSvc := auth.NewService(pool, []byte(secret))
	stockSvc := stock.NewService(pool)

	authH := auth.NewHandler(authSvc)
	stockH := stock.NewHandler(stockSvc)

	publicRegistrars := []router.RouteRegistrar{
		authH,
	}

	protectedRegistrars := []router.RouteRegistrar{
		stockH,
	}

	r := router.NewRouter(authSvc,
		publicRegistrars,
		protectedRegistrars)
	if err := r.Run(":8080"); err != nil {
		slog.Error("server", "error", err)
		os.Exit(1)
	}

}
