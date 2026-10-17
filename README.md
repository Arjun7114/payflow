markdown
# payflow

A typed, event-sourced payment-lifecycle state machine in Go, with a strictly
separated AI risk layer. Built to make illegal payment transitions impossible,
duplicate requests safe, and fraud scoring advisory-only — never in the
money-moving path.

## What it does

A payment moves through a lifecycle — `initiated → authorized → captured →
settled`, plus `refunded`, `failed`, `expired`, and an AI-requested
`flagged_hold`. payflow models this as a single pure transition table: a state
change is legal *only* if the table allows it. Illegal moves (double capture,
double refund, capturing a failed payment) are rejected by the core and cannot
move money.

## Architecture
      HTTP (net/http, no framework)
                 │
                 ▼
       Application service ──────── idempotency keys
                 │                  (retry-safe commands)
                 ▼
    Pure domain core (Go) ───────── the 9 legal transitions;
     deterministic, 0 deps          illegal states rejected
                 │
                 ▼
        Store interface
        ├── in-memory (mutex)
        └── Postgres (PK + UNIQUE constraints enforce concurrency)

    ── AI layer (Python, separate) ─────────────────────────
    Reads payment data; the ONLY action it can take is calling
    POST /payments/{id}/flag-hold through the audited API.
    It never moves money or writes to the ledger.

### The AI boundary (important)

The fraud layer is **advisory only**. It scores a transaction and, if risk is
high enough, requests a hold via `POST /payments/{id}/flag-hold` — the same
endpoint any client uses, producing an audited `authorized → flagged_hold`
transition the Go core validates. If the AI asks for an illegal hold, the core
refuses (HTTP 409). The deterministic core is always the sole authority over
money movement.

## Run it

Requires Go 1.22+ (stdlib router). Postgres and Python are optional.

```bash
# Start the service (in-memory store, listens on :8080)
go run ./cmd/payflow

# Walk a payment through its lifecycle
curl -X POST localhost:8080/payments -H "Idempotency-Key: k1" -d '{"payment_id":"p1"}'
curl -X POST localhost:8080/payments/p1/authorize -H "Idempotency-Key: k2"
curl -X POST localhost:8080/payments/p1/capture   -H "Idempotency-Key: k3"
curl localhost:8080/payments/p1
```

Every state-changing request requires an `Idempotency-Key` header, so retries
are safe. An illegal transition returns `409 Conflict`.

### Tests

```bash
go test ./...                 # unit + property tests; Postgres tests auto-skip
go test ./... -race           # same, under the race detector

# Postgres-backed tests (optional): start Postgres, then
#   docker run --name payflow-pg -e POSTGRES_PASSWORD=payflow \
#     -e POSTGRES_USER=payflow -e POSTGRES_DB=payflow -p 5432:5432 -d postgres:16
#   docker exec -i payflow-pg psql -U payflow -d payflow < db/schema.sql
DATABASE_URL=postgres://payflow:payflow@localhost:5432/payflow \
  go test ./internal/store -run TestPostgres -v
```

### AI fraud layer (optional)

```bash
cd ai
python -m venv .venv && .venv/bin/activate   # Windows: .venv\Scripts\activate
pip install -r requirements.txt
python -m payflow_ai.generate                # synthetic transactions
python -m payflow_ai.model                   # train the Isolation Forest
python -m payflow_ai.threshold               # inspect the precision/recall tradeoff

# With payflow running, score a payment and maybe hold it:
python -m payflow_ai.bridge --payment-id fraud-demo
```

The three demonstrated outcomes: a **risky** transaction is held
(`flagged_hold`); a **safe** one is ignored; an **illegal** hold request is
refused by the core (409).

## What's proven

See [METRICS.md](./METRICS.md) for the full list with reproduction commands.
Highlights (all reproducible from this repo):

- 8 states, 9 legal transitions; ~55 illegal (state,command) pairs rejected.
- 4 safety invariants property-tested across 2,000+ generated cases each.
- Idempotency + optimistic concurrency proven under 100 concurrent duplicate
  requests (exactly one effect), race-detector clean — enforced both in-memory
  (mutex) and by Postgres constraints.

The fraud model's metrics are on **synthetic data** and validate the pipeline,
not real-world fraud-detection performance.

## Status

- Core state machine, persistence, idempotency, REST API: complete.
- AI fraud scoring + audited hold bridge: complete.
- Rules-based failure triage, Haskell port of the pure core: in progress.