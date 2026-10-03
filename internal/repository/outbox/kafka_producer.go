package outbox

import (
	"context"
	"errors"
	"fmt"

	"github.com/segmentio/kafka-go"
)

// A topic that doesn't exist yet is created with these settings the first time a publish to it
// fails with UnknownTopicOrPartition. Deliberately minimal: it only fires for a topic nobody
// provisioned, and a topic that already exists is never touched, so ops can still provision
// topics with the partitions/replication they want ahead of time.
const (
	autoCreatePartitions        = 1
	autoCreateReplicationFactor = 1
)

// messageWriter is the part of *kafka.Writer the producer uses.
type messageWriter interface {
	WriteMessages(ctx context.Context, msgs ...kafka.Message) error
	Close() error
}

// topicCreator is the part of *kafka.Client the producer uses to create a missing topic.
type topicCreator interface {
	CreateTopics(ctx context.Context, req *kafka.CreateTopicsRequest) (*kafka.CreateTopicsResponse, error)
}

// KafkaProducer is the production Producer implementation.
type KafkaProducer struct {
	writer messageWriter
	admin  topicCreator
}

// NewKafkaProducer returns a Producer publishing to brokers. A single
// writer is reused across topics — the outbox relay passes the topic
// per-message via Message.Topic.
//
// The writer itself never auto-creates topics (AllowAutoTopicCreation is off, so a typo can't
// silently mint a topic); Produce creates a missing one explicitly instead — see Produce.
func NewKafkaProducer(brokers []string) *KafkaProducer {
	return &KafkaProducer{
		writer: &kafka.Writer{
			Addr:                   kafka.TCP(brokers...),
			Balancer:               &kafka.Hash{},
			RequiredAcks:           kafka.RequireAll,
			AllowAutoTopicCreation: false,
		},
		admin: &kafka.Client{Addr: kafka.TCP(brokers...)},
	}
}

// Produce implements Producer. If the topic doesn't exist on the broker it is created, then the
// publish is retried once: without that, one row for a never-provisioned topic fails forever and,
// because the relay publishes strictly in order, blocks every event queued behind it.
func (p *KafkaProducer) Produce(ctx context.Context, msg Message) error {
	headers := make([]kafka.Header, 0, len(msg.Headers))
	for k, v := range msg.Headers {
		headers = append(headers, kafka.Header{Key: k, Value: []byte(v)})
	}
	km := kafka.Message{
		Topic:   msg.Topic,
		Key:     msg.Key,
		Value:   msg.Value,
		Headers: headers,
	}

	err := p.writer.WriteMessages(ctx, km)
	if !isUnknownTopic(err) {
		return err
	}
	if createErr := p.createTopic(ctx, msg.Topic); createErr != nil {
		return fmt.Errorf("outbox: topic %q is missing and could not be created: %w (publish error: %v)", msg.Topic, createErr, err)
	}
	return p.writer.WriteMessages(ctx, km)
}

func (p *KafkaProducer) createTopic(ctx context.Context, topic string) error {
	resp, err := p.admin.CreateTopics(ctx, &kafka.CreateTopicsRequest{
		Topics: []kafka.TopicConfig{{
			Topic:             topic,
			NumPartitions:     autoCreatePartitions,
			ReplicationFactor: autoCreateReplicationFactor,
		}},
	})
	if err != nil {
		return err
	}
	// Another producer (or a parallel relay replica) may have created it between our failed
	// publish and now — that's the outcome we wanted.
	if topicErr := resp.Errors[topic]; topicErr != nil && !errors.Is(topicErr, kafka.TopicAlreadyExists) {
		return topicErr
	}
	return nil
}

// isUnknownTopic reports whether err means "the topic doesn't exist on the broker". The writer
// may hand the broker's error back directly or wrapped in kafka.WriteErrors (one entry per
// message), so both are checked.
func isUnknownTopic(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, kafka.UnknownTopicOrPartition) {
		return true
	}
	var writeErrors kafka.WriteErrors
	if errors.As(err, &writeErrors) {
		for _, e := range writeErrors {
			if errors.Is(e, kafka.UnknownTopicOrPartition) {
				return true
			}
		}
	}
	return false
}

// Close releases the underlying writer's connections.
func (p *KafkaProducer) Close() error {
	return p.writer.Close()
}
