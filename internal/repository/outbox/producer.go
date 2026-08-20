package outbox

import "context"

// Message is what Producer publishes: a single Kafka record with headers
// (used to carry the OTel trace context, per §2.2) plus the raw
// Protobuf-encoded payload.
type Message struct {
	Topic   string
	Key     []byte
	Value   []byte
	Headers map[string]string
}

//go:generate go run go.uber.org/mock/mockgen -destination=producer_mock.go -package=outbox . Producer

// Producer publishes a single message to Kafka. Defined here (accept
// interfaces, return structs) so Relay can be unit-tested against a
// generated mock instead of a real broker, per
// docs/microservice-standards.md §4.
type Producer interface {
	Produce(ctx context.Context, msg Message) error
}
