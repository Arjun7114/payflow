// Package domain contains the pure payment state machine: the states a
// payment can occupy, the commands that drive it, and the events it emits.
// Nothing here performs I/O, and no transition logic lives in this package
// yet — these files define only the vocabulary the rest of the system is
// built on.
package domain

// State is the lifecycle stage of a payment. It is backed by a string so it
// reads clearly in logs, JSON, and the event store.
type State string

const (
	// StateInitiated: payment created, not yet authorized.
	StateInitiated State = "initiated"
	// StateAuthorized: funds authorized by the issuer, not yet captured.
	StateAuthorized State = "authorized"
	// StateCaptured: authorized funds have been captured (charge committed).
	StateCaptured State = "captured"
	// StateSettled: captured funds have settled to the merchant.
	StateSettled State = "settled"
	// StateRefunded: captured or settled funds have been returned (terminal).
	StateRefunded State = "refunded"
	// StateFailed: authorization or capture failed (terminal).
	StateFailed State = "failed"
	// StateExpired: an authorization lapsed before capture (terminal).
	StateExpired State = "expired"
	// StateFlaggedHold: the AI risk layer requested a hold pending review.
	StateFlaggedHold State = "flagged_hold"
)

// AllStates lists every valid state. Property tests generate from this set.
var AllStates = []State{
	StateInitiated,
	StateAuthorized,
	StateCaptured,
	StateSettled,
	StateRefunded,
	StateFailed,
	StateExpired,
	StateFlaggedHold,
}

// Valid reports whether s is a known state.
func (s State) Valid() bool {
	for _, k := range AllStates {
		if s == k {
			return true
		}
	}
	return false
}