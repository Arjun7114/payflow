package domain

import "time"

// Event is an immutable record of a state change that has already happened to
// a payment. Events are appended to a per-payment log (the store, added next);
// that log is both the audit trail and the stream the AI layer reads. The
// genesis event (payment creation) has an empty From and To == StateInitiated.
type Event struct {
	PaymentID  string
	Sequence   int // 0-based position within this payment's log
	Type       EventType
	From       State
	To         State
	OccurredAt time.Time
}

// NewInitiationEvent builds the genesis event that creates a payment. Creation
// is not a transition (there is no prior state), so it does not go through
// Transition; it is the fixed entry point into the state machine.
func NewInitiationEvent(paymentID string, at time.Time) Event {
	return Event{
		PaymentID:  paymentID,
		Sequence:   0,
		Type:       EventInitiated,
		From:       "",
		To:         StateInitiated,
		OccurredAt: at,
	}
}

// CurrentState reconstructs a payment's state from its event log. Because each
// event records the state it moved the payment To, the current state is simply
// the last event's To. Returns ok=false for an empty log (no such payment).
func CurrentState(events []Event) (state State, ok bool) {
	if len(events) == 0 {
		return "", false
	}
	return events[len(events)-1].To, true
}
