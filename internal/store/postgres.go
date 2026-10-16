package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/Arjun7114/payflow/internal/domain"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres is a Store backed by a Postgres event table. It implements exactly
// the same interface as Memory, so the service and API are unchanged. Its
// correctness guarantees come from the database schema, not application locks:
//   - PRIMARY KEY (payment_id, sequence) rejects a second append at the same
//     sequence — the concurrency guarantee.
//   - UNIQUE (payment_id, idempotency_key) rejects a retried command — the
//     idempotency guarantee.
// A violation of either surfaces as a Postgres unique-violation, which Append
// maps back to the same ErrConcurrencyConflict / ErrDuplicateKey the in-memory
// store returns, so callers behave identically against either backend.
type Postgres struct {
	pool *pgxpool.Pool
}

// NewPostgres builds a Postgres store from an existing pool.
func NewPostgres(pool *pgxpool.Pool) *Postgres {
	return &Postgres{pool: pool}
}

// pgUniqueViolation is the SQLSTATE code Postgres returns for a unique or
// primary-key constraint violation.
const pgUniqueViolation = "23505"

// Load returns all events for a payment in sequence order, or ErrNotFound.
func (p *Postgres) Load(ctx context.Context, paymentID string) ([]domain.Event, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT payment_id, sequence, event_type, from_state, to_state, occurred_at
		   FROM payment_events
		  WHERE payment_id = $1
		  ORDER BY sequence`, paymentID)
	if err != nil {
		return nil, fmt.Errorf("load query: %w", err)
	}
	defer rows.Close()

	var events []domain.Event
	for rows.Next() {
		var e domain.Event
		if err := rows.Scan(&e.PaymentID, &e.Sequence, &e.Type, &e.From, &e.To, &e.OccurredAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}
	if len(events) == 0 {
		return nil, ErrNotFound
	}
	return events, nil
}

// Append inserts one event. It does not pre-check sequence or key: it simply
// inserts and lets the database constraints decide. A unique violation means
// either the sequence was taken (concurrency conflict) or the key was reused
// (duplicate). We distinguish them by which constraint fired. This "insert and
// catch" approach is race-free because the check is the write.
func (p *Postgres) Append(ctx context.Context, e domain.Event, expectedSeq int, idempotencyKey string) error {
	_, err := p.pool.Exec(ctx,
		`INSERT INTO payment_events
		   (payment_id, sequence, event_type, from_state, to_state, idempotency_key, occurred_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		e.PaymentID, expectedSeq, e.Type, e.From, e.To, idempotencyKey, e.OccurredAt)

	if err == nil {
		return nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		// Which constraint fired tells us which guarantee kicked in.
		switch pgErr.ConstraintName {
		case "payment_events_payment_id_idempotency_key_key":
			return ErrDuplicateKey
		case "payment_events_pkey":
			return ErrConcurrencyConflict
		default:
			// Any other unique violation is still a conflict at this position.
			return ErrConcurrencyConflict
		}
	}
	return fmt.Errorf("append insert: %w", err)
}

// SeenKey reports whether an idempotency key was already used for a payment.
func (p *Postgres) SeenKey(ctx context.Context, paymentID, idempotencyKey string) (bool, error) {
	var exists bool
	err := p.pool.QueryRow(ctx,
		`SELECT EXISTS (
		    SELECT 1 FROM payment_events
		     WHERE payment_id = $1 AND idempotency_key = $2)`,
		paymentID, idempotencyKey).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("seenkey query: %w", err)
	}
	return exists, nil
}

// ensure Postgres satisfies Store at compile time.
var _ Store = (*Postgres)(nil)
