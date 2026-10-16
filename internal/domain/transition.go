package domain

import (
	"errors"
	"fmt"
)

// ErrIllegalTransition is returned when a command is not valid from the
// payment's current state. Every money-moving decision in the system funnels
// through Transition, so this single error represents "this cannot happen to
// a payment in this state".
var ErrIllegalTransition = errors.New("illegal payment transition")

// transition is the outcome of a legal command: the resulting state and the
// event that records it.
type transition struct {
	next  State
	event EventType
}

// transitionTable is the single source of truth for the payment state machine.
// A (current state, command) pair is legal if and only if it has an entry
// here. These are exactly the nine edges in the lifecycle diagram. Every other
// pair — double capture, refund after refund, capturing a failed payment,
// any move out of a terminal state — is simply absent, and therefore rejected.
var transitionTable = map[State]map[Command]transition{
	StateInitiated: {
		CmdAuthorize: {next: StateAuthorized, event: EventAuthorized},
		CmdFail:      {next: StateFailed, event: EventFailed},
	},
	StateAuthorized: {
		CmdCapture:  {next: StateCaptured, event: EventCaptured},
		CmdExpire:   {next: StateExpired, event: EventExpired},
		CmdFlagHold: {next: StateFlaggedHold, event: EventHoldFlagged},
	},
	StateFlaggedHold: {
		CmdClearHold: {next: StateCaptured, event: EventHoldCleared},
	},
	StateCaptured: {
		CmdSettle: {next: StateSettled, event: EventSettled},
		CmdRefund: {next: StateRefunded, event: EventRefunded},
	},
	StateSettled: {
		CmdRefund: {next: StateRefunded, event: EventRefunded},
	},
}

// Transition applies cmd to a payment currently in `current`. It is the one
// place in payflow where a state change is decided: given a legal
// (state, command) pair it returns the next state and the event to record;
// given anything else it returns ErrIllegalTransition and the caller makes no
// change. Pure — no I/O, no mutation, no clock.
func Transition(current State, cmd Command) (State, EventType, error) {
	if !current.Valid() {
		return "", "", fmt.Errorf("%w: unknown state %q", ErrIllegalTransition, current)
	}
	cmds, ok := transitionTable[current]
	if !ok {
		// Valid but terminal (failed, expired, refunded): no outgoing commands.
		return "", "", fmt.Errorf("%w: %q is terminal, cannot apply %q", ErrIllegalTransition, current, cmd)
	}
	t, ok := cmds[cmd]
	if !ok {
		return "", "", fmt.Errorf("%w: cannot %q from %q", ErrIllegalTransition, cmd, current)
	}
	return t.next, t.event, nil
}

// IsTerminal reports whether s is an end state with no legal outgoing
// transitions (failed, expired, refunded).
func IsTerminal(s State) bool {
	_, ok := transitionTable[s]
	return s.Valid() && !ok
}
