# payflow — metrics

Every figure here is produced by this repository and reproducible with the
listed command. Rows are added as each phase lands. Nothing is projected.

Last updated: 2026-10-16 (Phase 2 — persistence, idempotency, concurrency proven)

## Core state machine (Phase 1)

| Metric | Value | How to reproduce |
|---|---|---|
| States modeled | 8 | `internal/domain/state.go` (`AllStates`) |
| Commands modeled | 8 | `internal/domain/command.go` (`AllCommands`) |
| Legal transitions | 9 | `internal/domain/transition.go` (table) |
| Illegal (state,command) pairs rejected | 55 / 64 | 8×8 − 9; verified by property "transition matches legal set" |
| Invariants property-tested | 4 | `go test ./internal/domain -v` |
| Generated cases passed per property | 2000 | `go test ./internal/domain -v` |
| Core runtime dependencies | 0 | `go list -deps ./internal/domain` (stdlib only) |

Invariants proven (domain):
1. A (state, command) pair is accepted iff it is one of the 9 legal edges;
   every other pair is rejected with ErrIllegalTransition.
2. Terminal states (failed, expired, refunded) accept no command.
3. Across random command histories, a payment is never captured more than once.
4. State reconstructed from the event log always equals the state produced by
   applying the same transitions directly (event-sourcing consistency).

## Persistence, idempotency & concurrency (Phase 2)

Proven at two layers: the in-memory store (serialized by a mutex) and a real
Postgres store (serialized by database constraints — no application lock).

| Metric | Value | How to reproduce |
|---|---|---|
| Concurrent duplicate requests (same key) → effects applied | 100 → 1 | `go test ./internal/app -race -run TestService_Concurrent -v` |
| Concurrent distinct-key captures → successes | 100 → 1 | same test file |
| In-memory store race (same sequence, N writers) → winners | 50 → 1 | `go test ./internal/store -race -v` |
| Postgres store race (same sequence, N writers) → commits | 50 → 1 | `DATABASE_URL=... go test ./internal/store -run TestPostgres -v` |
| Tests pass under Go race detector | yes | `go test ./... -race` |
| Concurrency tests re-run under stress | 20× clean | `go test ./internal/app -race -run TestService_Concurrent -count=20` |

Guarantees proven:
- Idempotency: N identical requests (same key) apply exactly one effect; the
  rest are detected as retries and replayed, never re-applied.
- Optimistic concurrency: N genuinely distinct requests racing to advance one
  payment yield exactly one success; the rest are rejected as conflicts.
- Database-enforced: against real Postgres, the same guarantees are enforced by
  PRIMARY KEY (payment_id, sequence) and UNIQUE (payment_id, idempotency_key) —
  the losing concurrent INSERTs are rejected by the database, with no
  application-level lock. The Postgres integration tests skip automatically when
  DATABASE_URL is unset, so the default suite needs no database.

## REST API (Phase 2)

| Metric | Value | How to reproduce |
|---|---|---|
| HTTP endpoints | 10 | `internal/api/api.go` (Routes) |
| Framework dependencies | 0 (net/http only) | `go list -deps ./internal/api` |
| Illegal transition over HTTP | 409 Conflict | `go test ./internal/api -v` |
| Missing idempotency key | 400 Bad Request | `go test ./internal/api -v` |

## AI fraud layer (Phase 3)

All figures below are on **synthetic data** with realistically overlapping
fraud/legit distributions (see `ai/payflow_ai/generate.py`). They validate the
end-to-end pipeline — generate → train → evaluate → threshold → act — and
demonstrate method and judgment. They are **not** claims about real-world
fraud-detection performance, where labels are scarce, fraud drifts, and feature
engineering dominates.

| Metric | Value | How to reproduce |
|---|---|---|
| Model | Isolation Forest (unsupervised) | `ai/payflow_ai/model.py` |
| Training uses labels | no (labels used only to evaluate) | `model.py` |
| ROC-AUC (ranking quality) | ~0.88 | `python -m payflow_ai.model` |
| Operating point | recall-leaning, threshold ≈ -0.041 | `python -m payflow_ai.threshold` |
| — precision at that point | ~0.18 | threshold sweep |
| — recall at that point | ~0.68 | threshold sweep |
| — share of traffic flagged | ~7.5% | threshold sweep |
| AI actions on the core | 1 only: request hold via audited API | `ai/payflow_ai/bridge.py` |

Design notes:
- **Threshold is a deliberate choice, not a default.** The sweep shows the full
  precision/recall tradeoff; the recall-leaning point reflects that in payments
  a missed fraud is a realized loss while a false hold is reviewable. A
  different business could justify a different point from the same curve.
- **The AI never moves money.** Its sole action is `POST /flag-hold`, which the
  Go core validates like any other request — demonstrated by three outcomes:
  risky → held, safe → ignored, illegal hold → refused by the core (HTTP 409).
- ROC-AUC ~0.88 on separable-but-overlapping synthetic data indicates the
  model ranks risk well; the low precision/recall at any single threshold is
  the expected consequence of a 2% base rate, not a defect.

  ## Haskell port of the pure core (Phase 4)

A port of the deterministic payment core to Haskell, where the type system
enforces at compile time what the Go version checks at runtime.

| Metric | Value | How to reproduce |
|---|---|---|
| States as a sum type (invalid state unrepresentable) | 8 constructors | `haskell-core/src/Payflow/State.hs` |
| Transition totality checked at compile time | yes (-Wincomplete-patterns) | `cd haskell-core && cabal build` |
| Property-tested invariants (Hedgehog) | 3 | `cd haskell-core && cabal test` |
| Generated cases per property | 100 | `cabal test` |

What the compiler guarantees here that Go cannot:
- A `PaymentState` value can only be one of the 8 declared states; an invalid
  state cannot be constructed. In Go, state is a string guarded at runtime.
- The `transition` function is total: because the library is built with
  `-Wincomplete-patterns`, GHC refuses to compile if any `(state, command)`
  case is left unhandled. Unhandled transitions are a build error, not a
  runtime bug.

The Hedgehog properties mirror the Go suite (legality, terminal-finality,
no double-capture), so both implementations are checked against the same laws.

markdown
## Latency (load test)

Measured by `cmd/loadtest` firing 2000 capture requests at 50 concurrent
workers against a locally running server. Client, server, and database all run
on one machine over localhost, so these are a RELATIVE measure (in-memory vs
Postgres, and a check that the handler path is fast) — not a claim about
production latency under real network conditions and load.

| Metric | In-memory store | Postgres store |
|---|---|---|
| Throughput | ~4,350 req/s | ~1,650 req/s |
| Latency p50 | ~0.5 ms | ~10 ms |
| Latency p95 | ~21 ms | ~12 ms |
| Latency p99 | ~26 ms | ~18 ms |
| Errors | 0 | 0 |

Reproduce:
terminal 1 — server (set DATABASE_URL for the Postgres run)

go run ./cmd/payflow

terminal 2

go run ./cmd/loadtest -n 2000 -c 50


Reading: the in-memory store has a faster median but a jittery tail (GC and
scheduler pauses); Postgres pays ~10 ms per request for a network round-trip and
a constraint-checked INSERT, but delivers a tighter, more predictable latency
distribution. Durability costs median latency and buys consistency.

I rounded to clean figures (~0.5 ms, ~10 ms) and kept the ~ because these shift run-to-run — honest hedging, consistent with how we’ve recorded everything.
