# Design decisions

This document records the reasoning behind payflow's main design choices — the
"why" that the code alone doesn't show. Each entry states the decision, the
alternatives considered, and the trade-off accepted.

## 1. A pure transition table, not scattered if/switch logic

The state machine is one `map[State]map[Command]transition` (Go) and one total
`transition` function (Haskell). A transition is legal if and only if it has an
entry; everything else is rejected.

**Why:** the table *is* the lifecycle diagram, in code — auditable at a glance
("show me every way a payment can move"). Control-flow scattered across
handlers hides the rules and makes illegal transitions easy to introduce by
omission.

**Trade-off:** a table is slightly less flexible than ad-hoc code for one-off
special cases — but payment lifecycles shouldn't have ad-hoc special cases.

## 2. Determinism in the money path; AI strictly advisory

No AI, randomness, or non-deterministic input touches the code that decides
whether a payment can change state. The ML fraud layer runs as a separate
process, reads events, and its only action on the core is requesting a hold
through the same audited API any client uses.

**Why:** money movement must be reproducible and auditable. An LLM or model
deciding a state transition would make outcomes non-reproducible and is a
red flag in a payments system. Keeping AI advisory means the system behaves
identically whether the AI is running or switched off.

**Trade-off:** the AI can't *act* directly, only recommend — which is exactly
the point. If the AI requests an illegal hold, the core refuses it (HTTP 409).

## 3. Event sourcing over a mutable status column

A payment's state is derived from an append-only event log, not stored as a
mutable field.

**Why:** the log is a free, complete audit trail — a hard requirement in
payments — and it's the stream the AI layer reads without coupling to the core.
Current state is just the last event's destination.

**Trade-off:** reading current state means reading the log (or a projection).
For this scope that's fine; at scale you'd add a cached projection.

## 4. Idempotency keys + optimistic concurrency, not locks held across work

Every state-changing command carries an idempotency key. The store appends an
event only if (a) the key is unused and (b) the log is at the expected
sequence — both checked atomically with the write.

**Why:** retries are a fact of life in payments (network timeouts, UPI callback
re-fires, double-clicks). The key makes a retried request a no-op; the sequence
check makes two genuinely-different concurrent requests resolve to exactly one
winner. "Check-then-write" under optimistic control is race-free because the
check *is* the write — no lock is held across business logic.

**Trade-off:** a losing concurrent request must retry. That's correct behaviour,
not a cost.

## 5. Same guarantee enforced at two layers (mutex and database)

Idempotency/concurrency are proven against both an in-memory store (serialized
by a mutex) and Postgres (serialized by `PRIMARY KEY` and `UNIQUE` constraints).

**Why:** the in-memory version makes the logic fast to test under `-race`; the
Postgres version proves the guarantee holds with no shared application lock,
enforced by the database itself. Claiming only one would overstate or
under-demonstrate.

**Trade-off:** two implementations of one interface to maintain — justified by
the clean ports-and-adapters boundary it required.

## 6. Standard library over frameworks (net/http, no web framework)

The HTTP layer uses Go's standard `net/http` with its 1.22+ method/path router.
No Gin, Echo, or Chi.

**Why:** the core already has zero runtime dependencies; keeping the API
dependency-free is a deliberate strength. The stdlib router covers everything
this API needs. Fewer dependencies, less to audit, nothing to break on upgrade.

**Trade-off:** a framework offers conveniences (middleware ecosystems, etc.)
we forgo. For an API this size, not needed.

## 7. Unsupervised fraud model (Isolation Forest), evaluated on synthetic data

The fraud scorer is an unsupervised Isolation Forest. Labels are used only to
evaluate, never to train. All data is synthetic and documented as such.

**Why:** real fraud labels are scarce, late (chargebacks arrive weeks later),
and incomplete — so an unsupervised model that learns "normal" and flags
outliers reflects how this works in production. Synthetic data, clearly labeled,
validates the pipeline (generate → train → evaluate → threshold → act) without
pretending to measure real-world performance.

**Trade-off:** metrics don't describe real fraud. This is stated plainly
everywhere. The pipeline is designed so real data is a drop-in replacement.

## 8. A recall-leaning fraud threshold, chosen from a sweep

The operating point (score ≈ -0.041) is chosen deliberately for higher recall
at the cost of precision, rather than maximizing F1.

**Why:** in payments, a missed fraud is a realized financial loss; a false hold
is a reviewable inconvenience (or a step-up verification). The costs are
asymmetric, so F1 (which weights precision and recall equally) is the wrong
objective. The threshold sweep exposes the full trade-off; the chosen point
reflects the business cost, and a different business could justify a different
point from the same curve.

**Trade-off:** more false holds, needing review capacity. That's the right
trade when missing fraud is more expensive.

## 9. Rules-based failure triage, not an LLM

Failure explanations come from a deterministic rules engine over the event
history, not a model.

**Why:** the set of failure reasons in a bounded state machine is small and
known, so rules are correct, auditable, and need no dependencies or API keys.
ML is reserved for the problem that actually needs it (fraud). An optional LLM
layer can rephrase the same facts, never override them.

**Trade-off:** less flexible phrasing than an LLM — acceptable, since the facts
are fixed.

## 10. A Haskell port of the pure core

The pure state machine is ported to Haskell, where states are a sum type and
the `transition` function is total.

**Why:** it proves the type-safety thesis the project is built on. In Go,
"illegal states are rejected" is a runtime check over strings. In Haskell, an
invalid `PaymentState` cannot be *constructed*, and `-Wincomplete-patterns`
makes an unhandled transition a *compile error*. The guarantee moves from
runtime to compile time. Hedgehog property tests mirror the Go suite, so both
implementations are checked against the same laws.

**Trade-off:** a second implementation of the core to keep in sync — justified
by the compile-time guarantee it demonstrates.