package store

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/Arjun7114/payflow/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

// newPostgresForTest connects to the database named by DATABASE_URL and returns
// a Postgres store plus a cleanup function. If DATABASE_URL is unset the test is
// skipped — so the default `go test ./...` run never requires a live database.
func newPostgresForTest(t *testing.T) (*Postgres, context.Context) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping Postgres integration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	// Start each test from a clean table and clean up afterward.
	truncate := func() {
		if _, err := pool.Exec(ctx, `TRUNCATE payment_events`); err != nil {
			t.Fatalf("truncate: %v", err)
		}
	}
	truncate()
	t.Cleanup(truncate)

	return NewPostgres(pool), ctx
}

func pgEvent(id string, seq int) domain.Event {
	return domain.Event{
		PaymentID:  id,
		Sequence:   seq,
		Type:       domain.EventInitiated,
		From:       "",
		To:         domain.StateInitiated,
		OccurredAt: time.Unix(1_700_000_000, 0).UTC(),
	}
}

// A plain append then load round-trips through Postgres.
func TestPostgres_AppendThenLoad(t *testing.T) {
	p, ctx := newPostgresForTest(t)

	if err := p.Append(ctx, pgEvent("p1", 0), 0, "k0"); err != nil {
		t.Fatalf("append: %v", err)
	}
	events, err := p.Load(ctx, "p1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(events) != 1 || events[0].To != domain.StateInitiated {
		t.Fatalf("unexpected log: %+v", events)
	}
}

// A reused idempotency key is rejected by the UNIQUE(payment_id, key) constraint.
func TestPostgres_DuplicateKeyRejected(t *testing.T) {
	p, ctx := newPostgresForTest(t)

	if err := p.Append(ctx, pgEvent("p1", 0), 0, "same"); err != nil {
		t.Fatalf("first append: %v", err)
	}
	if err := p.Append(ctx, pgEvent("p1", 1), 1, "same"); err != ErrDuplicateKey {
		t.Fatalf("got %v, want ErrDuplicateKey", err)
	}
}

// A second append at the same sequence is rejected by the primary key.
func TestPostgres_StaleSequenceRejected(t *testing.T) {
	p, ctx := newPostgresForTest(t)

	if err := p.Append(ctx, pgEvent("p1", 0), 0, "k0"); err != nil {
		t.Fatalf("first append: %v", err)
	}
	// Different key, but sequence 0 is already taken.
	if err := p.Append(ctx, pgEvent("p1", 0), 0, "k1"); err != ErrConcurrencyConflict {
		t.Fatalf("got %v, want ErrConcurrencyConflict", err)
	}
}

// THE METRIC — DB-enforced. N goroutines race to append the SAME sequence with
// DISTINCT keys against real Postgres. Exactly one INSERT commits; the rest are
// rejected by the primary-key constraint. No application lock is involved — the
// database is the serialization point.
func TestPostgres_ConcurrentAppendExactlyOneWins(t *testing.T) {
	p, ctx := newPostgresForTest(t)
	const racers = 50

	start := make(chan struct{})
	results := make(chan error, racers)

	for i := 0; i < racers; i++ {
		key := "k" + strconv.Itoa(i)
		go func() {
			<-start
			results <- p.Append(ctx, pgEvent("race", 0), 0, key)
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
		t.Fatalf("%d concurrent inserts committed, want exactly 1", wins)
	}

	events, err := p.Load(ctx, "race")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("log has %d events, want 1", len(events))
	}
}
