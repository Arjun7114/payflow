package app

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Arjun7114/payflow/internal/domain"
	"github.com/Arjun7114/payflow/internal/store"
)

// fixedClock gives deterministic timestamps so tests never depend on wall time.
func fixedClock() time.Time { return time.Unix(1_700_000_000, 0) }

// newAuthorizedPayment creates a payment and authorizes it, returning a service
// whose single payment "p" sits in StateAuthorized — the setup for capture.
func newAuthorizedPayment(t *testing.T) (*Service, context.Context) {
	t.Helper()
	svc := New(store.NewMemory(), fixedClock)
	ctx := context.Background()

	if _, err := svc.Create(ctx, "p", "create-key"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Handle(ctx, "p", domain.CmdAuthorize, "auth-key"); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	return svc, ctx
}

// Basic happy path: create -> authorize -> capture -> settle.
func TestService_HappyPath(t *testing.T) {
	svc, ctx := newAuthorizedPayment(t)
	if r, err := svc.Handle(ctx, "p", domain.CmdCapture, "cap"); err != nil || r.State != domain.StateCaptured {
		t.Fatalf("capture: state=%q err=%v", r.State, err)
	}
	if r, err := svc.Handle(ctx, "p", domain.CmdSettle, "set"); err != nil || r.State != domain.StateSettled {
		t.Fatalf("settle: state=%q err=%v", r.State, err)
	}
}

// An illegal command is rejected by the pure core, surfaced through the service.
func TestService_IllegalCommandRejected(t *testing.T) {
	svc, ctx := newAuthorizedPayment(t)
	// Settle directly from Authorized (must go through Captured first) is illegal.
	if _, err := svc.Handle(ctx, "p", domain.CmdSettle, "bad"); err == nil {
		t.Fatal("expected illegal transition, got nil")
	}
}

// Sequential retry with the SAME key is a no-op: capture once, "capture" again
// with the same key returns the prior result and does not double-apply.
func TestService_SameKeyIsIdempotent(t *testing.T) {
	svc, ctx := newAuthorizedPayment(t)

	r1, err := svc.Handle(ctx, "p", domain.CmdCapture, "cap-key")
	if err != nil || r1.State != domain.StateCaptured || r1.Replayed {
		t.Fatalf("first capture: %+v err=%v", r1, err)
	}
	r2, err := svc.Handle(ctx, "p", domain.CmdCapture, "cap-key")
	if err != nil {
		t.Fatalf("retry errored: %v", err)
	}
	if !r2.Replayed || r2.State != domain.StateCaptured {
		t.Fatalf("retry should replay captured state, got %+v", r2)
	}
}

// THE METRIC — duplicate requests under concurrency.
// Fire N identical capture requests (same key) at once. Exactly one applies a
// capture; the rest are detected as retries. The payment is captured once, and
// its event log grows by exactly one event.
func TestService_ConcurrentDuplicatesApplyOnce(t *testing.T) {
	svc, ctx := newAuthorizedPayment(t)
	const N = 100

	var wg sync.WaitGroup
	start := make(chan struct{})
	applied := make([]bool, N) // applied[i] = this request actually moved the state

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			r, err := svc.Handle(ctx, "p", domain.CmdCapture, "same-key")
			if err != nil {
				t.Errorf("goroutine %d: unexpected error %v", i, err)
				return
			}
			applied[i] = !r.Replayed // only the one real application is non-replayed
		}(i)
	}
	close(start)
	wg.Wait()

	applications := 0
	for _, a := range applied {
		if a {
			applications++
		}
	}
	if applications != 1 {
		t.Fatalf("%d requests applied the capture, want exactly 1", applications)
	}

	// The money moved once: querying state shows Captured, and a settle still
	// works (proving we're in a clean Captured state, not a corrupted one).
	r, err := svc.Handle(ctx, "p", domain.CmdSettle, "settle-after")
	if err != nil || r.State != domain.StateSettled {
		t.Fatalf("post-capture settle failed: state=%q err=%v", r.State, err)
	}
}

// SECOND DANGER — distinct keys, concurrent, on a payment that can only advance
// once. Fire N captures each with a different key. Exactly one succeeds; the
// others lose the optimistic sequence check (ErrConcurrencyConflict). This is
// the "no double spend from genuinely separate requests" guarantee.
func TestService_ConcurrentDistinctKeysApplyOnce(t *testing.T) {
	svc, ctx := newAuthorizedPayment(t)
	const N = 100

	var wg sync.WaitGroup
	start := make(chan struct{})
	okCount := make([]bool, N)

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			r, err := svc.Handle(ctx, "p", domain.CmdCapture, "key-"+strconv.Itoa(i))
			if err == nil && !r.Replayed {
				okCount[i] = true
			}
		}(i)
	}
	close(start)
	wg.Wait()

	wins := 0
	for _, ok := range okCount {
		if ok {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("%d distinct-key captures succeeded, want exactly 1", wins)
	}
}
