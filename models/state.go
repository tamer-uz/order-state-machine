package models

import "time"

type State string
type Event string

type Transition struct {
	from  State
	event Event
}

const (
	Initialized       State = "initialized"
	PaymentAuthorized State = "payment_authorized"
	Complete          State = "complete"
	Rejected          State = "rejected"
	VoidPending       State = "void_pending"
	Cancelled         State = "cancelled"
	NeedsAttention    State = "needs_attention"

	PaymentSucceeded    Event = "payment_succeeded"
	PaymentFailed       Event = "payment_failed"
	CompletionSucceeded Event = "completion_succeeded"
	CompletionFailed    Event = "completion_failed"
	VoidSucceeded       Event = "void_succeeded"
	VoidFailed          Event = "void_failed"
)

var Transitions = map[Transition]State{
	{Initialized, PaymentSucceeded}:          PaymentAuthorized,
	{Initialized, PaymentFailed}:             Rejected,
	{PaymentAuthorized, CompletionSucceeded}: Complete,
	{PaymentAuthorized, CompletionFailed}:    VoidPending,
	{VoidPending, VoidSucceeded}:             Cancelled,
	{VoidPending, VoidFailed}:                NeedsAttention,
}

type TransitionEntry struct {
	State     State     `json:"state"`
	Timestamp time.Time `json:"timestamp"`
	Reason    string    `json:"reason,omitempty"`
}
