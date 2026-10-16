# payflow

A typed payment-lifecycle state machine with a deterministic core and a
separate AI risk layer.

payflow models a payment as a state machine (initiated → authorized →
captured → settled, plus refunded / failed / expired / flagged-hold).
Illegal transitions — double capture, double refund, capturing a failed
payment — are rejected by the core and cannot move money. The core is
deterministic and auditable: no AI, no randomness touches the money-moving path.

## Design boundary

- **Deterministic core** owns every state transition and all money movement.
- **AI layer** (fraud scoring + failure triage) only *reads* the event stream.
  The one thing it may write is a request for an `authorized → flagged-hold`
  transition, through the same guarded API as any other caller — logged and
  reversible. It never mutates the ledger directly.

## Stack

- Core: Go — Postgres append-only event log, idempotent commands
- Pure domain ported to Haskell, property-tested with Hedgehog
- AI layer: Python — Isolation Forest risk scoring, RAG failure explainer

## Status

In progress, built step by step. See the commit history.