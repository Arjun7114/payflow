// Package store defines the persistence boundary for payflow: an append-only
// log of domain events per payment. The application service depends only on
// the Store interface, never on a concrete database, so implementations
// (in-memory, Postgres) are interchangeable. The append-with-expected-sequence
// contract is what makes concurrent command handling safe.
package store

import (
	"context"
	"errors"

	"github.com/Arjun7114/payflow/internal/domain"
)

// ErrConcurrencyConflict is returned by Append when the log's current length
// does not match the caller's expected sequence — i.e. another writer advanced
// this payment first. The caller should reload and decide again.
var ErrConcurrencyConflict = errors.New("store: concurrency conflict")

// ErrDuplicateKey is returned by Append when the idempotency key was already
// used for this payment — the command is a retry and must not be re-applied.
var ErrDuplicateKey = errors.New("store: duplicate idempotency key")

// ErrNotFound is returned when no events exist for a payment ID.
var ErrNotFound = errors.New("store: payment not found")

// Store is an append-only event log keyed by payment ID.
// Store is an append-only event log keyed by payment ID, plus per-payment
// idempotency-key bookkeeping so a retried command is applied at most once.
type Store interface {
	// Load returns all events for a payment, in sequence order, or ErrNotFound.
	Load(ctx context.Context, paymentID string) ([]domain.Event, error)

	// Append atomically adds one event to a payment's log — but only if the log
	// currently holds exactly expectedSeq events, and only if idempotencyKey has
	// not already been used for this payment. The sequence check and the key
	// check happen together with the write:
	//   - stale sequence        -> ErrConcurrencyConflict (a genuine conflict)
	//   - key already seen       -> ErrDuplicateKey        (a retried request)
	// On ErrDuplicateKey nothing is written and the earlier effect stands.
	Append(ctx context.Context, e domain.Event, expectedSeq int, idempotencyKey string) error

	// SeenKey reports whether idempotencyKey was already applied to this payment.
	SeenKey(ctx context.Context, paymentID, idempotencyKey string) (bool, error)
}
