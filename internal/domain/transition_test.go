package domain

import (
	"errors"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// legalEdges is the set of (from, cmd) pairs that MUST succeed, written out by
// hand so the test does not just re-import the table it is meant to check. If
// the transition table changes, this list must change to match.
var legalEdges = []struct {
	from State
	cmd  Command
	to   State
}{
	{StateInitiated, CmdAuthorize, StateAuthorized},
	{StateInitiated, CmdFail, StateFailed},
	{StateAuthorized, CmdCapture, StateCaptured},
	{StateAuthorized, CmdExpire, StateExpired},
	{StateAuthorized, CmdFlagHold, StateFlaggedHold},
	{StateFlaggedHold, CmdClearHold, StateCaptured},
	{StateCaptured, CmdSettle, StateSettled},
	{StateCaptured, CmdRefund, StateRefunded},
	{StateSettled, CmdRefund, StateRefunded},
}

func legal(from State, cmd Command) bool {
	for _, e := range legalEdges {
		if e.from == from && e.cmd == cmd {
			return true
		}
	}
	return false
}

// TestLegalEdgesSucceed is a plain table test: every edge we declare legal
// must be accepted by Transition and land in the expected state.
func TestLegalEdgesSucceed(t *testing.T) {
	for _, e := range legalEdges {
		next, event, err := Transition(e.from, e.cmd)
		if err != nil {
			t.Errorf("legal edge %q --%q--> rejected: %v", e.from, e.cmd, err)
			continue
		}
		if next != e.to {
			t.Errorf("edge %q --%q-->: got state %q, want %q", e.from, e.cmd, next, e.to)
		}
		if event == "" {
			t.Errorf("edge %q --%q--> produced empty event", e.from, e.cmd)
		}
	}
}

// genState and genCommand draw uniformly from the full vocabulary.
func genState() gopter.Gen   { return gen.OneConstOf(toAny(AllStates)...) }
func genCommand() gopter.Gen { return gen.OneConstOf(toAnyCmd(AllCommands)...) }

func toAny(xs []State) []interface{} {
	out := make([]interface{}, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}
func toAnyCmd(xs []Command) []interface{} {
	out := make([]interface{}, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}

// TestProperties runs all three invariants under gopter.
func TestProperties(t *testing.T) {
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = 2000 // generated cases per property
	props := gopter.NewProperties(params)

	// Property 1 — exhaustive legality. For every (state, command), Transition
	// agrees with the declared legal set: legal pairs succeed; all others are
	// rejected with ErrIllegalTransition.
	props.Property("transition matches legal set", prop.ForAll(
		func(from State, cmd Command) bool {
			next, event, err := Transition(from, cmd)
			if legal(from, cmd) {
				return err == nil && next.Valid() && event != ""
			}
			return err != nil && errors.Is(err, ErrIllegalTransition)
		},
		genState(), genCommand(),
	))

	// Property 2 — terminal states are final. No command moves a payment out of
	// failed, expired, or refunded. Forbids double-refund and capture-after-fail.
	props.Property("terminal states are final", prop.ForAll(
		func(s State, cmd Command) bool {
			if !IsTerminal(s) {
				return true // vacuously true for non-terminal states
			}
			_, _, err := Transition(s, cmd)
			return err != nil
		},
		genState(), genCommand(),
	))

	// Property 3 — no double-capture. Walk a random legal history from
	// Initiated; a payment is never captured more than once.
	props.Property("no double capture over random histories", prop.ForAll(
		func(cmds []Command) bool {
			state := StateInitiated
			captures := 0
			for _, cmd := range cmds {
				next, _, err := Transition(state, cmd)
				if err != nil {
					continue // illegal from here; state unchanged
				}
				if next == StateCaptured && state != StateCaptured {
					captures++
				}
				state = next
			}
			return captures <= 1
		},
		gen.SliceOf(genCommand()),
	))

	props.TestingRun(t)
}
