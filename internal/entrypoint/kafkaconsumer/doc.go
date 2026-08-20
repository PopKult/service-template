// Package kafkaconsumer holds this service's inbound Kafka consumers.
// Wire go-common/middleware/kafkamw.Extract to continue the publishing
// request's trace, keep the handler thin (unmarshal -> call exactly one
// use case -> ack/nack), and make sure it's idempotent — the outbox
// relay's polling can redeliver a message. See
// docs/microservice-standards.md §2.2 and §3.1 in the repo root.
//
// Nothing lives here yet — this is a template. Add a Consumer once the
// service has a topic to consume, matching this shape so it can be
// passed straight to go-common/shutdown.Wait alongside the gRPC server:
//
//	type Consumer struct{ ... }
//	func (c *Consumer) Start(ctx context.Context) error
//	func (c *Consumer) Stop(ctx context.Context)
package kafkaconsumer
