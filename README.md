pubsub
=========

Simple pubsub framework for Go.

This is (should be, fingers crossed) safe to be used from multiple goroutines. Designed such that only one goroutine makes changes to the object structure


```go
var svc pubsub.Service

var msgs []interface{}
// You can create your own subscriber, of course, but this
// is the built-in hack to allow closures (eek)
sub := pubsub.SubscribeFunc(func(v interface{}) {
  msgs = append(msgs, v)
})

// Subscribing before starting the main loop is safe
svc.Subscribe(sub)

// Sending before starting the main loop is safe
svc.Send(`Hello`)

ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
defer cancel()

// Start the main loop
go svc.Run(ctx)

// Sending after starting the main loop.. is obviously safe.
svc.Send(`World!`)

// If you have another subscriber, you can add it here.
// It will only receive subsequent pubsub requests
// svc.Subscribe(sub2)
```
