package domain

// EventType names a fact that has already happened to a payment. A successful
// command produces exactly one event, which (from the store phase onward) is
// appended to an immutable log — the audit trail and the stream the AI layer
// reads. This file defines only the vocabulary; the full event record and the
// command-to-event mapping arrive with the transition function and the store.
type EventType string

const (
	EventInitiated   EventType = "payment_initiated"
	EventAuthorized  EventType = "payment_authorized"
	EventCaptured    EventType = "payment_captured"
	EventSettled     EventType = "payment_settled"
	EventRefunded    EventType = "payment_refunded"
	EventFailed      EventType = "payment_failed"
	EventExpired     EventType = "payment_expired"
	EventHoldFlagged EventType = "payment_hold_flagged"
	EventHoldCleared EventType = "payment_hold_cleared"
)

// AllEventTypes lists every event the core can emit.
var AllEventTypes = []EventType{
	EventInitiated,
	EventAuthorized,
	EventCaptured,
	EventSettled,
	EventRefunded,
	EventFailed,
	EventExpired,
	EventHoldFlagged,
	EventHoldCleared,
}