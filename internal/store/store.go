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

// ErrNotFound is returned when no events exist for a payment ID.
var ErrNotFound = errors.New("store: payment not found")

// Store is an append-only event log keyed by payment ID.
type Store interface {
	// Load returns all events for a payment, in sequence order. If the payment
	// has no events it returns ErrNotFound.
	Load(ctx context.Context, paymentID string) ([]domain.Event, error)

	// Append atomically adds one event to a payment's log, but only if the log
	// currently holds exactly expectedSeq events (so the new event becomes
	// sequence expectedSeq). If the log has moved on, it returns
	// ErrConcurrencyConflict and appends nothing. This optimistic check is the
	// serialization point for concurrent writers.
	Append(ctx context.Context, e domain.Event, expectedSeq int) error
}
