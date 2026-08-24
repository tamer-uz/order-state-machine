package models

import (
	"errors"
	"testing"
)

func TestProcessTransitionValid(t *testing.T) {
	cases := []struct {
		from  State
		event Event
		want  State
	}{
		{Initialized, PaymentSucceeded, PaymentAuthorized},
		{Initialized, PaymentFailed, Rejected},
		{PaymentAuthorized, CompletionSucceeded, Complete},
		{PaymentAuthorized, CompletionFailed, VoidPending},
		{VoidPending, VoidSucceeded, Cancelled},
		{VoidPending, VoidFailed, NeedsAttention},
	}

	if len(Transitions) != len(cases) {
		t.Fatalf("transition table has %d entries, expected %d", len(Transitions), len(cases))
	}

	for _, c := range cases {
		t.Run(string(c.from)+"/"+string(c.event), func(t *testing.T) {
			order := Order{CurrentState: c.from}

			if err := order.ProcessTransition(c.event, nil); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if order.CurrentState != c.want {
				t.Errorf("state = %q, want %q", order.CurrentState, c.want)
			}
			if len(order.StateTransitionHistory) != 1 {
				t.Fatalf("history has %d entries, want 1", len(order.StateTransitionHistory))
			}
			if got := order.StateTransitionHistory[0].State; got != c.want {
				t.Errorf("history state = %q, want %q", got, c.want)
			}
		})
	}
}

func TestProcessTransitionTerminalStatesRejectEverything(t *testing.T) {
	terminal := []State{Complete, Rejected, Cancelled, NeedsAttention}
	events := []Event{
		PaymentSucceeded, PaymentFailed,
		CompletionSucceeded, CompletionFailed,
		VoidSucceeded, VoidFailed,
	}

	for _, state := range terminal {
		for _, event := range events {
			t.Run(string(state)+"/"+string(event), func(t *testing.T) {
				order := Order{CurrentState: state}

				if err := order.ProcessTransition(event, nil); err == nil {
					t.Fatalf("expected error, got none")
				}
				if order.CurrentState != state {
					t.Errorf("state changed to %q, want %q", order.CurrentState, state)
				}
				if len(order.StateTransitionHistory) != 0 {
					t.Errorf("history gained %d entries on a rejected transition",
						len(order.StateTransitionHistory))
				}
			})
		}
	}
}

func TestProcessTransitionRecordsReason(t *testing.T) {
	cause := errors.New("void failed at provider")
	order := Order{CurrentState: VoidPending}

	if err := order.ProcessTransition(VoidFailed, cause); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entry := order.StateTransitionHistory[0]
	if entry.Reason != cause.Error() {
		t.Errorf("reason = %q, want %q", entry.Reason, cause.Error())
	}
}

func TestProcessTransitionOmitsReasonOnSuccess(t *testing.T) {
	order := Order{CurrentState: Initialized}

	if err := order.ProcessTransition(PaymentSucceeded, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reason := order.StateTransitionHistory[0].Reason; reason != "" {
		t.Errorf("reason = %q, want empty", reason)
	}
}
