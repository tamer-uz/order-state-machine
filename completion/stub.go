package completion

import "errors"

var ErrCompletionFailed = errors.New("order completion failed")

type Stub struct {
	Fails bool
}

func (s *Stub) Complete(orderID string) error {
	if s.Fails {
		return ErrCompletionFailed
	}
	return nil
}
