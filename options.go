package broadcast

import "github.com/lestrrat-go/option"

type Option = option.Interface
type identAck struct{}
type identBackend struct{}

type CommandOption interface {
	Option
	commandOption()
}

type commandOption struct {
	Option
}

func (*commandOption) commandOption() {}

type RunOption interface {
	Option
	runOption()
}

type runOption struct {
	Option
}

func (*runOption) runOption() {}

// WithAck specifies if the command method such as Subscribe()
// should block to wait for an ack from the Service
func WithAck(b bool) CommandOption {
	return &commandOption{option.New(identAck{}, b)}
}

// WithBackend specifies the backend object to be used by the
// service.
func WithBackend(b Backend) RunOption {
	return &runOption{option.New(identBackend{}, b)}
}
