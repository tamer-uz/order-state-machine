package storage

import (
	"fmt"
	"sync"

	"github.com/tamer-uz/order-state-machine/models"
)

type Store struct {
	mu     sync.Mutex
	orders map[string]models.Order
}

func New() *Store {
	return &Store{
		orders: make(map[string]models.Order),
	}
}

func (s *Store) GetOrder(id string) (models.Order, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, found := s.orders[id]
	return o, found
}

func (s *Store) UpsertOrder(order models.Order) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orders[order.ID] = order
}

// SaveIfStateIs compares the stored state against expected and writes under the
// same lock, so the caller's read and its write cannot be interleaved.
func (s *Store) SaveIfStateIs(expected models.State, order models.Order) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, found := s.orders[order.ID]
	if !found {
		// Orders are never deleted, so a caller holding one that is gone is a bug.
		return fmt.Errorf("save order %s: not found", order.ID)
	}
	if stored.CurrentState != expected {
		return fmt.Errorf("save order %s: state is %q, want %q: %w",
			order.ID, stored.CurrentState, expected, models.ErrStateChanged)
	}

	s.orders[order.ID] = order
	return nil
}
