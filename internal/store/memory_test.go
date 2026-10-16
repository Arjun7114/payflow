package store

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/Arjun7114/payflow/internal/domain"
)

func genesis(id string) domain.Event { return domain.NewInitiationEvent(id, time.Unix(0, 0)) }

// Load on an unknown payment returns ErrNotFound.
func TestMemory_LoadNotFound(t *testing.T) {
	m := NewMemory()
	if _, err := m.Load(context.Background(), "nope"); err != ErrNotFound {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

// A correct append at the expected sequence succeeds and is readable back.
func TestMemory_AppendThenLoad(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	if err := m.Append(ctx, genesis("p1"), 0, "k"); err != nil {
		t.Fatalf("append seq 0: %v", err)
	}
	events, err := m.Load(ctx, "p1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(events) != 1 || events[0].To != domain.StateInitiated {
		t.Fatalf("unexpected log: %+v", events)
	}
}

// Appending with a stale expected sequence is rejected.
func TestMemory_StaleSequenceRejected(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	_ = m.Append(ctx, genesis("p1"), 0, "k0") // log now length 1

	// Another writer, different key, still thinks the log is empty.
	err := m.Append(ctx, genesis("p1"), 0, "k1")
	if err != ErrConcurrencyConflict {
		t.Fatalf("got %v, want ErrConcurrencyConflict", err)
	}
}

// A reused idempotency key is rejected as a duplicate.
func TestMemory_DuplicateKeyRejected(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	if err := m.Append(ctx, genesis("p1"), 0, "same"); err != nil {
		t.Fatalf("first append: %v", err)
	}
	// Same key again — even at a valid next sequence — must be refused.
	e := genesis("p1")
	e.Sequence = 1
	if err := m.Append(ctx, e, 1, "same"); err != ErrDuplicateKey {
		t.Fatalf("got %v, want ErrDuplicateKey", err)
	}
}

// The core store guarantee: when many goroutines race to append the SAME
// sequence with DISTINCT keys, exactly one succeeds. This isolates the
// optimistic sequence check (not key dedup) as the serialization point.
func TestMemory_ConcurrentAppendExactlyOneWins(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	const racers = 50

	start := make(chan struct{})
	results := make(chan error, racers)

	for i := 0; i < racers; i++ {
		key := "k" + strconv.Itoa(i)
		go func() {
			<-start
			results <- m.Append(ctx, genesis("race"), 0, key)
		}()
	}
	close(start)

	wins := 0
	for i := 0; i < racers; i++ {
		if err := <-results; err == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("%d goroutines succeeded, want exactly 1", wins)
	}
	events, _ := m.Load(ctx, "race")
	if len(events) != 1 {
		t.Fatalf("log has %d events, want 1", len(events))
	}
}
