package pubsub

type Egress interface {
	Send(interface{}) error
}

// nilEgress does nothing
type nilEgress struct{}

func (nilEgress) Send(v interface{}) error {
	return nil
}
