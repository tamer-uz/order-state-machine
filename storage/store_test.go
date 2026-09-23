package storage

import (
	"errors"
	"testing"

	"github.com/tamer-uz/order-state-machine/models"
)

func seedOrder(t *testing.T, s *Store, state models.State) models.Order {
	t.Helper()

	order := models.NewOrder(5000)
	order.CurrentState = state
	s.UpsertOrder(order)
	return order
}

func TestSaveIfStateIsWritesWhenStateMatches(t *testing.T) {
	s := New()
	order := seedOrder(t, s, models.Initialized)

	order.CurrentState = models.Authorizing
	if err := s.SaveIfStateIs(models.Initialized, order); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stored, found := s.GetOrder(order.ID)
	if !found {
		t.Fatalf("order %s missing after save", order.ID)
	}
	if stored.CurrentState != models.Authorizing {
		t.Errorf("stored state = %q, want %q", stored.CurrentState, models.Authorizing)
	}
}

func TestSaveIfStateIsRejectsWhenStateMoved(t *testing.T) {
	s := New()
	order := seedOrder(t, s, models.Authorizing)

	// A caller that read the order while it was still initialized.
	stale := order
	stale.CurrentState = models.PaymentAuthorized

	err := s.SaveIfStateIs(models.Initialized, stale)

	if !errors.Is(err, models.ErrStateChanged) {
		t.Fatalf("error = %v, want %v", err, models.ErrStateChanged)
	}
	stored, _ := s.GetOrder(order.ID)
	if stored.CurrentState != models.Authorizing {
		t.Errorf("stored state = %q, want %q untouched", stored.CurrentState, models.Authorizing)
	}
}

func TestSaveIfStateIsRejectsUnknownOrder(t *testing.T) {
	s := New()

	err := s.SaveIfStateIs(models.Initialized, models.NewOrder(5000))

	if err == nil {
		t.Fatal("expected error, got none")
	}
	// A vanished order is our bug, not a lost race, so it is not ErrStateChanged.
	if errors.Is(err, models.ErrStateChanged) {
		t.Errorf("error = %v, want a non-ErrStateChanged error", err)
	}
}
