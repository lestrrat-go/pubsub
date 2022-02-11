package pubsub

import "github.com/lestrrat-go/option"

type Option = option.Interface
type identAck struct{}

type CommandOption interface {
	Option
	commandOption()
}

type commandOption struct {
	Option
}

func (*commandOption) commandOption() {}

// WithAck specifies if the command method such as Subscribe()
// should block to wait for an ack from the Service
func WithAck(b bool) CommandOption {
	return &commandOption{option.New(identAck{}, b)}
}
