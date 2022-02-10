package broadcast

import (
	"context"
)

type Backend interface {
	Send(interface{}) error
	Run(context.Context, *Service)
}

// nilBackend does nothing
type nilBackend struct{}

func (nilBackend) Send(v interface{}) error {
	return nil
}

func (nilBackend) Run(ctx context.Context, _ *Service) {
	<-ctx.Done()
}
