package storage

import (
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
