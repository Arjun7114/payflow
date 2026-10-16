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

## Planned (not yet measured)

- API latency p50 / p95 — load test
- Compile-time rejection of illegal transitions — Haskell port
- Fraud model precision / recall on synthetic data — Phase 3 (AI)