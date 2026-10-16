package domain

import (
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Property — event-sourcing consistency. Reconstructing a payment's state from
// its event log always equals the state produced by applying the same
// transitions directly. This ties the event log to the proven core: the log
// can never disagree with the state machine.
func TestProperties_EventSourcing(t *testing.T) {
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = 2000
	props := gopter.NewProperties(params)

	props.Property("reconstructed state equals transitioned state", prop.ForAll(
		func(cmds []Command) bool {
			state := StateInitiated
			events := []Event{NewInitiationEvent("p", time.Unix(0, 0))}

			for i, cmd := range cmds {
				next, evt, err := Transition(state, cmd)
				if err != nil {
					continue // illegal from here; log unchanged
				}
				events = append(events, Event{
					PaymentID:  "p",
					Sequence:   len(events),
					Type:       evt,
					From:       state,
					To:         next,
					OccurredAt: time.Unix(int64(i+1), 0),
				})
				state = next
			}

			got, ok := CurrentState(events)
			return ok && got == state
		},
		gen.SliceOf(genCommand()),
	))

	props.TestingRun(t)
}
