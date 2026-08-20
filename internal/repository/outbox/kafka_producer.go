package outbox

import (
	"context"

	"github.com/segmentio/kafka-go"
)

// KafkaProducer is the production Producer implementation.
type KafkaProducer struct {
	writer *kafka.Writer
}

// NewKafkaProducer returns a Producer publishing to brokers. A single
// writer is reused across topics — the outbox relay passes the topic
// per-message via Message.Topic.
func NewKafkaProducer(brokers []string) *KafkaProducer {
	return &KafkaProducer{
		writer: &kafka.Writer{
			Addr:                   kafka.TCP(brokers...),
			Balancer:               &kafka.Hash{},
			RequiredAcks:           kafka.RequireAll,
			AllowAutoTopicCreation: false,
		},
	}
}

// Produce implements Producer.
func (p *KafkaProducer) Produce(ctx context.Context, msg Message) error {
	headers := make([]kafka.Header, 0, len(msg.Headers))
	for k, v := range msg.Headers {
		headers = append(headers, kafka.Header{Key: k, Value: []byte(v)})
	}
	return p.writer.WriteMessages(ctx, kafka.Message{
		Topic:   msg.Topic,
		Key:     msg.Key,
		Value:   msg.Value,
		Headers: headers,
	})
}

// Close releases the underlying writer's connections.
func (p *KafkaProducer) Close() error {
	return p.writer.Close()
}
