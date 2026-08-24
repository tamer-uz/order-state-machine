package api

import "github.com/tamer-uz/order-state-machine/models"

// OrderStore persists orders.
type OrderStore interface {
	GetOrder(id string) (models.Order, bool)
	UpsertOrder(o models.Order)
}

// PaymentProvider is the external payment system.
type PaymentProvider interface {
	Authorize(orderID string, cents int) error
	Void(orderID string) error
}

// CompletionProvider fulfills the order after payment is authorized.
type CompletionProvider interface {
	Complete(orderID string) error
}
