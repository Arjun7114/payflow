# payflow

A typed, event-sourced payment-lifecycle engine in Go, with a strictly
separated advisory AI layer and a Haskell port of the core that makes illegal
states unrepresentable at compile time.

payflow models a payment as a state machine and is built around one principle:
**the money-moving core is deterministic, typed, and auditable; AI is advisory
only and can never move money.** Illegal transitions (double capture, double
refund, capturing a failed payment) are rejected by the core and cannot occur.

---

## What it does

A payment moves through a lifecycle:

`initiated → authorized → captured → settled`

plus `refunded`, `failed`, `expired`, and an AI-requested `flagged_hold`. The
legal transitions are defined as a single pure table; a state change happens
only if the table allows it. Every state change is recorded as an immutable
event, so a payment's history is its own audit trail.

## Why it's built this way

- **Determinism in the money path.** The core that decides whether a payment
  can move from one state to another is pure Go with zero runtime dependencies,
  property-tested, and auditable. No AI, no randomness, touches it.
- **AI strictly advisory.** A separate Python service scores transactions for
  fraud and can *request* a hold — but only through the same audited API any
  client uses. It cannot mutate the ledger or invent a transition.
- **The type-safety thesis, proven twice.** The Go core rejects illegal
  transitions at runtime; a Haskell port of the same core rejects them at
  *compile time* via sum types and exhaustive pattern matching.

## Architecture
        HTTP  (net/http, no framework)
                   │
                   ▼
        Application service ───────── idempotency keys (retry-safe)
                   │
                   ▼
      Pure domain core (Go) ───────── 9 legal transitions;
       deterministic, 0 deps          illegal moves rejected
                   │
                   ▼
         Store interface
         ├── in-memory  (mutex)
         └── Postgres   (PK + UNIQUE constraints enforce concurrency)

── AI layer (Python, separate process) ─────────────────────────────
Reads payment data; its ONLY action is POST /payments/{id}/flag-hold
through the audited API. Never moves money, never writes the ledger.
• Fraud scorer — Isolation Forest (unsupervised), synthetic data
• Failure triage — rules-based explainer (optional LLM layer)

── Haskell port (haskell-core/) ────────────────────────────────────
The pure core as sum types; illegal states are unrepresentable and
transition totality is checked at compile time (-Wincomplete-patterns).


## Repository layout

payflow/
cmd/payflow/ # runnable HTTP server
internal/
domain/ # pure state machine: states, transitions, events (0 deps)
store/ # Store interface + in-memory and Postgres adapters
app/ # application service: load → transition → append, idempotent
api/ # REST API over net/http
db/schema.sql # Postgres event table (PK + UNIQUE constraints)
ai/ # Python advisory layer (separate process)
payflow_ai/
generate.py # seeded synthetic transaction generator
model.py # Isolation Forest fraud scorer + evaluation
threshold.py # precision/recall threshold sweep
bridge.py # scores a payment, requests a hold via the API
triage.py # rules-based failure explainer
tests/
haskell-core/ # Haskell port of the pure core
src/Payflow/State.hs
test/Spec.hs # Hedgehog property tests
METRICS.md # every metric, with reproduction commands


## The payment lifecycle

| From \ Command | authorize | capture | settle | refund | expire | fail | flag_hold | clear_hold |
|---|---|---|---|---|---|---|---|---|
| initiated | authorized | — | — | — | — | failed | — | — |
| authorized | — | captured | — | — | expired | — | flagged_hold | — |
| flagged_hold | — | — | — | — | — | — | — | captured |
| captured | — | — | settled | refunded | — | — | — | — |
| settled | — | — | — | refunded | — | — | — | — |

Terminal states (`refunded`, `failed`, `expired`) have no outgoing transitions.
Every other (state, command) pair is rejected.

## Running it

Requires Go 1.22+. Postgres, Python, and Haskell are each optional.

### The service

```bash
go run ./cmd/payflow            # in-memory store, listens on :8080

curl -X POST localhost:8080/payments            -H "Idempotency-Key: k1" -d '{"payment_id":"p1"}'
curl -X POST localhost:8080/payments/p1/authorize -H "Idempotency-Key: k2"
curl -X POST localhost:8080/payments/p1/capture   -H "Idempotency-Key: k3"
curl localhost:8080/payments/p1
```

Every state-changing request requires an `Idempotency-Key` header, so retries
are safe. An illegal transition returns `409 Conflict`.

### Tests

```bash
go test ./...          # unit + property tests; Postgres tests auto-skip
go test ./... -race    # same, under the race detector
```

### Postgres (optional)

```bash
docker run --name payflow-pg -e POSTGRES_PASSWORD=payflow \
  -e POSTGRES_USER=payflow -e POSTGRES_DB=payflow -p 5432:5432 -d postgres:16
docker exec -i payflow-pg psql -U payflow -d payflow < db/schema.sql

DATABASE_URL=postgres://payflow:payflow@localhost:5432/payflow \
  go test ./internal/store -run TestPostgres -v
```

### AI fraud layer (optional)

```bash
cd ai
python -m venv .venv && .venv\Scripts\activate   # macOS/Linux: source .venv/bin/activate
pip install -r requirements.txt

python -m payflow_ai.generate     # synthetic transactions
python -m payflow_ai.model        # train the Isolation Forest
python -m payflow_ai.threshold    # inspect the precision/recall tradeoff
python -m payflow_ai.triage       # rules-based failure explanations

# with the service running, score a payment and maybe hold it:
python -m payflow_ai.bridge --payment-id fraud-demo
```

Three demonstrated outcomes: a **risky** transaction is held (`flagged_hold`);
a **safe** one is ignored; an **illegal** hold request is refused by the core
(`409`).

### Haskell port (optional)

```bash
cd haskell-core
cabal build        # compiles only if transition handling is exhaustive
cabal test         # Hedgehog property tests
```

## What's proven

Full list with reproduction commands in [METRICS.md](./METRICS.md).

**Core (Go)**
- 8 states, 9 legal transitions; ~55 illegal (state,command) pairs rejected.
- 4 safety invariants property-tested across 2,000 generated cases each.
- Core has zero runtime dependencies.

**Persistence & concurrency**
- Idempotency + optimistic concurrency proven under 100 concurrent duplicate
  requests (exactly one effect), race-detector clean.
- Enforced both in-memory (mutex) and by Postgres constraints (PK + UNIQUE).

**API**
- 10 REST endpoints, zero web-framework dependencies (net/http only).

**AI layer**
- Unsupervised Isolation Forest, ROC-AUC ≈ 0.88, with a deliberate
  recall-leaning operating point chosen from a threshold sweep.
- Rules-based failure triage.
- The AI's only action on the core is an audited hold request.

**Haskell port**
- Payment states as a sum type: invalid states are unrepresentable.
- Transition totality enforced at compile time (`-Wincomplete-patterns`).
- 3 Hedgehog property tests mirroring the Go invariants.

## Honest scope note

The fraud model is trained and evaluated on **synthetic data** (see
`ai/payflow_ai/generate.py`). Its metrics validate the end-to-end pipeline and
the method; they are **not** claims about real-world fraud-detection
performance, where labels are scarce, fraud drifts, and feature engineering
dominates. The pipeline is designed so real transaction data is a drop-in
replacement for the generator.

## Status

All phases complete:

- ✅ Pure core (property-tested state machine)
- ✅ Persistence, idempotency, concurrency (in-memory + Postgres)
- ✅ REST API
- ✅ AI fraud scorer + audited hold bridge
- ✅ Rules-based failure triage
- ✅ Haskell port with compile-time guarantees

## Tech stack

Go · PostgreSQL (pgx) · gopter (property testing) · Python (scikit-learn,
pandas) · Haskell (GHC, Cabal, Hedgehog) · Docker