package main

import (
	"context"
	"log"
	"os"
	"time"

	"warehouse-manager/internal/auth"
	"warehouse-manager/internal/db"
	"warehouse-manager/internal/router"
	"warehouse-manager/internal/stock"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()
	log.Println("database connected")

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		log.Fatal("JWT_SECRET not set")
	}

	if len(secret) < 32 {
		log.Fatal("JWT_SECRET invalid length")
	}

	authSvc := auth.NewService(pool, []byte(secret))
	stockSvc := stock.NewService(pool)

	authH := auth.NewHandler(authSvc)
	stockH := stock.NewHandler(stockSvc)

	r := router.NewRouter(stockH, authH)
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("server: %v", err)
	}

}
