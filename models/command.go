package models

type Command string

const (
	AuthorizePayment Command = "authorize_payment"
	CompleteOrder    Command = "complete_order"
)

var ValidCommands = map[Command]bool{
	AuthorizePayment: true,
	CompleteOrder:    true,
}
