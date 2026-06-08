package outbox

import "context"

type Relay struct{}

func NewRelay() *Relay {
	return &Relay{}
}

func (r *Relay) Run(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}
