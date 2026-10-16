package store

import (
	"context"
	"sync"

	"github.com/Arjun7114/payflow/internal/domain"
)

// Memory is an in-memory Store. It is safe for concurrent use: a single mutex
// guards all state, and the idempotency-key check, the optimistic sequence
// check, and the append all happen inside one critical section — so neither a
// retry nor a concurrent writer can slip through. This models exactly what a
// Postgres unique constraint on (payment_id, sequence) and a stored
// idempotency key enforce at the database level later.
type Memory struct {
	mu   sync.Mutex
	logs map[string][]domain.Event
	keys map[string]map[string]bool // paymentID -> set of used idempotency keys
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{
		logs: make(map[string][]domain.Event),
		keys: make(map[string]map[string]bool),
	}
}

// Load returns a copy of the payment's events so callers cannot mutate the
// store's internal slice. Returns ErrNotFound for an unknown payment.
func (m *Memory) Load(ctx context.Context, paymentID string) ([]domain.Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	events, ok := m.logs[paymentID]
	if !ok || len(events) == 0 {
		return nil, ErrNotFound
	}
	out := make([]domain.Event, len(events))
	copy(out, events)
	return out, nil
}

// Append enforces both guarantees in one critical section: the idempotency-key
// check and the optimistic sequence check happen together with the write, so
// neither a retry nor a concurrent writer can slip through.
func (m *Memory) Append(ctx context.Context, e domain.Event, expectedSeq int, idempotencyKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.keys[e.PaymentID][idempotencyKey] {
		return ErrDuplicateKey
	}
	if len(m.logs[e.PaymentID]) != expectedSeq {
		return ErrConcurrencyConflict
	}

	m.logs[e.PaymentID] = append(m.logs[e.PaymentID], e)
	if m.keys[e.PaymentID] == nil {
		m.keys[e.PaymentID] = make(map[string]bool)
	}
	m.keys[e.PaymentID][idempotencyKey] = true
	return nil
}

// SeenKey reports whether this key was already applied to this payment.
func (m *Memory) SeenKey(ctx context.Context, paymentID, idempotencyKey string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.keys[paymentID][idempotencyKey], nil
}
