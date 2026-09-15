package main

import (
	"context"
	"log"
	"os"
	"time"

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

	stockSvc := stock.NewService(pool)
	stockH := stock.NewHandler(stockSvc)

	r := router.NewRouter(stockH)
	r.Run(":8080")

}
