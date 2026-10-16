# payflow — metrics

Every figure here is produced by this repository and reproducible with the
listed command. Rows are added as each phase lands. Nothing is projected.

Last updated: 2026-10-16 (Phase 1 — pure core complete)

## Core state machine (Phase 1)

| Metric | Value | How to reproduce |
|---|---|---|
| States modeled | 8 | see `internal/domain/state.go` (`AllStates`) |
| Commands modeled | 8 | see `internal/domain/command.go` (`AllCommands`) |
| Legal transitions | 9 | see `internal/domain/transition.go` (table) |
| Illegal (state,command) pairs rejected | 55 / 64 | 8×8 − 9; verified by property "transition matches legal set" |
| Invariants property-tested | 3 | `go test ./internal/domain -v` |
| Generated cases passed per property | 2000 | `go test ./internal/domain -v` |
| Core runtime dependencies | 0 | `go list -deps ./internal/domain` (stdlib only) |

Invariants proven:
1. Transition accepts a (state, command) pair if and only if it is one of the
   9 legal edges; every other pair is rejected with ErrIllegalTransition.
2. Terminal states (failed, expired, refunded) accept no command — no
   double-refund, no capture-after-failure.
3. Across random command histories, a payment is never captured more than once.

## Planned (not yet measured)

- Idempotency under N concurrent duplicate requests — Phase 2
- API latency p50 / p95 — Phase 2
- Compile-time rejection of illegal transitions — Haskell port
- Fraud model precision / recall on synthetic data — Phase 3