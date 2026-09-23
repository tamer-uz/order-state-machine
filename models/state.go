package models

import (
	"errors"
	"time"
)

type State string
type Event string

// ErrStateChanged reports that a store's copy of an order was not in the state
// the caller required. It lives here, not in storage, so the api package can
// recognise it without importing a concrete store.
var ErrStateChanged = errors.New("order state changed")

type Transition struct {
	from  State
	event Event
}

const (
	Initialized       State = "initialized"
	Authorizing       State = "authorizing"
	PaymentAuthorized State = "payment_authorized"
	Complete          State = "complete"
	Rejected          State = "rejected"
	VoidPending       State = "void_pending"
	Cancelled         State = "cancelled"
	NeedsAttention    State = "needs_attention"

	EnteringAuthorization Event = "entering_authorization"
	PaymentSucceeded      Event = "payment_succeeded"
	PaymentFailed         Event = "payment_failed"
	CompletionSucceeded   Event = "completion_succeeded"
	CompletionFailed      Event = "completion_failed"
	VoidSucceeded         Event = "void_succeeded"
	VoidFailed            Event = "void_failed"
)

// entering_authorization is the one event not derived from a provider result:
// it records the claim that keeps a second request away from the provider.
var Transitions = map[Transition]State{
	{Initialized, EnteringAuthorization}:     Authorizing,
	{Authorizing, PaymentSucceeded}:          PaymentAuthorized,
	{Authorizing, PaymentFailed}:             Rejected,
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
