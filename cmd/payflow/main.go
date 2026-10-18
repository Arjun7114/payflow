// Command payflow runs the payment service over HTTP. It wires the in-memory
// store and the app service to the API, and listens on :8080 (override with
// the PORT environment variable). The Postgres store will be swapped in here
// later without touching the API or service.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/Arjun7114/payflow/internal/api"
	"github.com/Arjun7114/payflow/internal/app"
	"github.com/Arjun7114/payflow/internal/store"
)

func main() {
	svc := app.New(store.NewMemory(), nil)
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
