package payment

import "errors"

var (
	ErrDeclined   = errors.New("payment declined by provider")
	ErrVoidFailed = errors.New("void failed at provider")
)

type Stub struct {
	AuthorizeFails bool
	VoidFails      bool
}

func (s *Stub) Authorize(orderID string, cents int) error {
	if s.AuthorizeFails {
		return ErrDeclined
	}
	return nil
}

func (s *Stub) Void(orderID string) error {
	if s.VoidFails {
		return ErrVoidFailed
	}
	return nil
}
