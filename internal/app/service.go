// Package app is the application service: it turns a command against a payment
// into the load -> transition -> append cycle, enforcing idempotency. It is the
// only layer that combines the pure domain core with a Store; the HTTP layer
// (added later) calls into here and does no business logic of its own.
package app

import (
	"context"
	"errors"
	"time"

	"github.com/Arjun7114/payflow/internal/domain"
	"github.com/Arjun7114/payflow/internal/store"
)

// Clock lets tests inject deterministic time. Production uses time.Now.
type Clock func() time.Time

// Service handles payment commands against a Store.
type Service struct {
	store store.Store
	now   Clock
}

// New builds a Service. If now is nil, time.Now is used.
func New(s store.Store, now Clock) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: s, now: now}
}

// Result is the outcome of handling a command.
type Result struct {
	State    domain.State
	Event    domain.EventType
	Replayed bool // true if this was a retried (idempotent) request, not re-applied
}

// Create initiates a new payment. The idempotency key guards against a retried
// creation producing two payments.
func (s *Service) Create(ctx context.Context, paymentID, idempotencyKey string) (Result, error) {
	if seen, _ := s.store.SeenKey(ctx, paymentID, idempotencyKey); seen {
		return s.replay(ctx, paymentID)
	}
	evt := domain.NewInitiationEvent(paymentID, s.now())
	err := s.store.Append(ctx, evt, 0, idempotencyKey)
	switch {
	case errors.Is(err, store.ErrDuplicateKey):
		return s.replay(ctx, paymentID)
	case err != nil:
		return Result{}, err
	}
	return Result{State: domain.StateInitiated, Event: domain.EventInitiated}, nil
}

// Handle applies a command to an existing payment. It is the heart of the
// service: load the log, ask the pure core whether the command is legal, and
// append the resulting event under the optimistic sequence check. A retried
// request (same key) is detected and replayed, never re-applied.
func (s *Service) Handle(ctx context.Context, paymentID string, cmd domain.Command, idempotencyKey string) (Result, error) {
	// Fast path: a key we've already applied is a retry — return prior outcome.
	if seen, _ := s.store.SeenKey(ctx, paymentID, idempotencyKey); seen {
		return s.replay(ctx, paymentID)
	}

	events, err := s.store.Load(ctx, paymentID)
	if err != nil {
		return Result{}, err
	}
	current, ok := domain.CurrentState(events)
	if !ok {
		return Result{}, store.ErrNotFound
	}

	// The pure core decides legality. No money logic lives here.
	// The pure core decides legality. No money logic lives here.
	next, evtType, err := domain.Transition(current, cmd)
	if err != nil {
		// A concurrent duplicate with THIS key may have committed between our
		// SeenKey check and our Load, advancing the state so the command is now
		// illegal. Before surfacing the error, re-check our own key: if it
		// already applied, this is that same request losing the race — replay
		// it, don't report a spurious illegal transition. Only a key we have
		// genuinely never applied yields ErrIllegalTransition.
		if seen, _ := s.store.SeenKey(ctx, paymentID, idempotencyKey); seen {
			return s.replay(ctx, paymentID)
		}
		return Result{}, err
	}

	evt := domain.Event{
		PaymentID:  paymentID,
		Sequence:   len(events),
		Type:       evtType,
		From:       current,
		To:         next,
		OccurredAt: s.now(),
	}

	err = s.store.Append(ctx, evt, len(events), idempotencyKey)
	switch {
	case errors.Is(err, store.ErrDuplicateKey):
		// A concurrent retry with the same key beat us here — replay its result.
		return s.replay(ctx, paymentID)
	case errors.Is(err, store.ErrConcurrencyConflict):
		// A different command advanced this payment first — a real conflict.
		return Result{}, err
	case err != nil:
		return Result{}, err
	}
	return Result{State: next, Event: evtType}, nil
}

// replay reconstructs the current state for a retried request without applying
// anything. The outcome is marked Replayed so callers can see it was a no-op.
func (s *Service) replay(ctx context.Context, paymentID string) (Result, error) {
	events, err := s.store.Load(ctx, paymentID)
	if err != nil {
		return Result{}, err
	}
	state, ok := domain.CurrentState(events)
	if !ok {
		return Result{}, store.ErrNotFound
	}
	last := events[len(events)-1]
	return Result{State: state, Event: last.Type, Replayed: true}, nil
}

// Status returns a payment's current state without changing anything.
func (s *Service) Status(ctx context.Context, paymentID string) (domain.State, error) {
	events, err := s.store.Load(ctx, paymentID)
	if err != nil {
		return "", err
	}
	state, ok := domain.CurrentState(events)
	if !ok {
		return "", store.ErrNotFound
	}
	return state, nil
}
