package pubsub

// Loopback is used for in-memory broadcasting and debugging.
// Whatever you send to it, it loops back to the pubsub.Service
type Loopback struct {
	svc *Service
}

func NewLoopback(svc *Service) *Loopback {
	return &Loopback{
		svc: svc,
	}
}

func (l *Loopback) Send(v interface{}) error {
	return l.svc.Receive(v)
}
