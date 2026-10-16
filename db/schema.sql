-- payflow event store schema.
--
-- A payment's history is an append-only sequence of events. State is never
-- stored directly; it is derived from the latest event (see domain.CurrentState).
--
-- Two constraints carry the correctness guarantees:
--   * UNIQUE (payment_id, sequence) makes concurrent appends at the same
--     position impossible — the database rejects the loser of any race. This is
--     optimistic concurrency control enforced by Postgres, not by application
--     locking.
--   * UNIQUE (payment_id, idempotency_key) makes a retried command a no-op at
--     the storage layer: the second insert with the same key cannot commit.

CREATE TABLE IF NOT EXISTS payment_events (
    payment_id      TEXT        NOT NULL,
    sequence        INTEGER     NOT NULL,
    event_type      TEXT        NOT NULL,
    from_state      TEXT        NOT NULL,
    to_state        TEXT        NOT NULL,
    idempotency_key TEXT        NOT NULL,
    occurred_at     TIMESTAMPTZ NOT NULL,

    PRIMARY KEY (payment_id, sequence),
    UNIQUE (payment_id, idempotency_key)
);

-- Supports loading a payment's full history in order.
CREATE INDEX IF NOT EXISTS idx_payment_events_payment
    ON payment_events (payment_id, sequence);