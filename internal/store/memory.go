package store

import (
	"context"
	"sync"

	"github.com/Arjun7114/payflow/internal/domain"
)

// Memory is an in-memory Store. It is safe for concurrent use: a single mutex
// guards the whole map, and critically, the expected-sequence check and the
// append happen inside the same locked section — so two goroutines racing on
// the same payment cannot both succeed at the same sequence. One wins; the
// other gets ErrConcurrencyConflict. This models exactly what a Postgres
// unique constraint on (payment_id, sequence) enforces at the DB level later.
type Memory struct {
	mu   sync.Mutex
	logs map[string][]domain.Event
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{logs: make(map[string][]domain.Event)}
}

// Load returns a copy of the payment's events so callers cannot mutate the
// store's internal slice.
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

// Append enforces the optimistic-concurrency contract: it writes e only if the
// current log length equals expectedSeq. The check and the write are a single
// critical section, so the decision cannot be raced.
func (m *Memory) Append(ctx context.Context, e domain.Event, expectedSeq int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.logs[e.PaymentID]) != expectedSeq {
		return ErrConcurrencyConflict
	}
	m.logs[e.PaymentID] = append(m.logs[e.PaymentID], e)
	return nil
}
