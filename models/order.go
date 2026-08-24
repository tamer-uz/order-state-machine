package models

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

type Order struct {
	ID                     string            `json:"id"`
	AmountCents            int               `json:"amount_cents"`
	CurrentState           State             `json:"current_state"`
	StateTransitionHistory []TransitionEntry `json:"history"`
}

func NewOrder(amountCents int) Order {
	return Order{
		ID:           generateOrderID(),
		AmountCents:  amountCents,
		CurrentState: Initialized,
		StateTransitionHistory: []TransitionEntry{
			{State: Initialized, Timestamp: time.Now()},
		},
	}
}

func (o *Order) ProcessTransition(event Event, cause error) error {
	next, ok := Transitions[Transition{o.CurrentState, event}]
	if !ok {
		return fmt.Errorf("cannot process %s from state %s", event, o.CurrentState)
	}

	entry := TransitionEntry{State: next, Timestamp: time.Now()}
	if cause != nil {
		entry.Reason = cause.Error()
	}

	o.CurrentState = next
	o.StateTransitionHistory = append(o.StateTransitionHistory, entry)
	return nil
}

func generateOrderID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}
