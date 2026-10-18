// Command payflow runs the payment service over HTTP. It selects a store at
// startup: if DATABASE_URL is set it uses Postgres (durable, enforced by the
// database's constraints); otherwise it falls back to the in-memory store, so
// the server runs with zero setup. The API and service layers are identical
// either way — only the store implementation differs.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Arjun7114/payflow/internal/api"
	"github.com/Arjun7114/payflow/internal/app"
	"github.com/Arjun7114/payflow/internal/store"
)

func main() {
	st, cleanup := selectStore()
	defer cleanup()

	svc := app.New(st, nil)
	handler := api.NewServer(svc).Routes()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	log.Printf("payflow listening on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// selectStore picks the store based on DATABASE_URL. It returns the store and a
// cleanup function to call on shutdown (a no-op for the in-memory store).
func selectStore() (store.Store, func()) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Printf("DATABASE_URL not set; using in-memory store (data is not persisted)")
		return store.NewMemory(), func() {}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		log.Fatalf("connecting to Postgres: %v", err)
	}
	// Fail fast if the database is unreachable, rather than on the first request.
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("pinging Postgres: %v", err)
	}

	log.Printf("using Postgres store")
	return store.NewPostgres(pool), pool.Close
}
