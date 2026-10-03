package outbox

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/segmentio/kafka-go"
)

type fakeWriter struct {
	results []error // one per WriteMessages call; the last one repeats
	calls   int
}

func (w *fakeWriter) WriteMessages(_ context.Context, _ ...kafka.Message) error {
	i := w.calls
	if i >= len(w.results) {
		i = len(w.results) - 1
	}
	w.calls++
	return w.results[i]
}

func (w *fakeWriter) Close() error { return nil }

type fakeAdmin struct {
	resp    *kafka.CreateTopicsResponse
	err     error
	created []kafka.TopicConfig
}

func (a *fakeAdmin) CreateTopics(_ context.Context, req *kafka.CreateTopicsRequest) (*kafka.CreateTopicsResponse, error) {
	a.created = append(a.created, req.Topics...)
	if a.err != nil {
		return nil, a.err
	}
	if a.resp != nil {
		return a.resp, nil
	}
	return &kafka.CreateTopicsResponse{Errors: map[string]error{}}, nil
}

var msg = Message{Topic: "juicer.word.added.v1", Key: []byte("k"), Value: []byte("v")}

func TestProduce_success_doesNotTouchTopics(t *testing.T) {
	w, a := &fakeWriter{results: []error{nil}}, &fakeAdmin{}

	if err := (&KafkaProducer{writer: w, admin: a}).Produce(context.Background(), msg); err != nil {
		t.Fatalf("Produce: %v", err)
	}
	if len(a.created) != 0 || w.calls != 1 {
		t.Errorf("created=%v writes=%d, want none and 1", a.created, w.calls)
	}
}

func TestProduce_unknownTopic_createsItAndRetriesOnce(t *testing.T) {
	w, a := &fakeWriter{results: []error{kafka.UnknownTopicOrPartition, nil}}, &fakeAdmin{}

	if err := (&KafkaProducer{writer: w, admin: a}).Produce(context.Background(), msg); err != nil {
		t.Fatalf("Produce: %v", err)
	}
	want := kafka.TopicConfig{Topic: msg.Topic, NumPartitions: autoCreatePartitions, ReplicationFactor: autoCreateReplicationFactor}
	if len(a.created) != 1 || a.created[0].Topic != want.Topic ||
		a.created[0].NumPartitions != want.NumPartitions || a.created[0].ReplicationFactor != want.ReplicationFactor {
		t.Errorf("created = %+v, want [%+v]", a.created, want)
	}
	if w.calls != 2 {
		t.Errorf("writes = %d, want 2 (failed publish + one retry)", w.calls)
	}
}

func TestProduce_unknownTopicWrappedInWriteErrors_isRecognised(t *testing.T) {
	w := &fakeWriter{results: []error{kafka.WriteErrors{kafka.UnknownTopicOrPartition}, nil}}
	a := &fakeAdmin{}

	if err := (&KafkaProducer{writer: w, admin: a}).Produce(context.Background(), msg); err != nil {
		t.Fatalf("Produce: %v", err)
	}
	if len(a.created) != 1 {
		t.Errorf("created = %v, want the topic created once", a.created)
	}
}

func TestProduce_topicCreatedByAnotherProducerMeanwhile_isFine(t *testing.T) {
	w := &fakeWriter{results: []error{kafka.UnknownTopicOrPartition, nil}}
	a := &fakeAdmin{resp: &kafka.CreateTopicsResponse{Errors: map[string]error{msg.Topic: kafka.TopicAlreadyExists}}}

	if err := (&KafkaProducer{writer: w, admin: a}).Produce(context.Background(), msg); err != nil {
		t.Fatalf("Produce: %v", err)
	}
	if w.calls != 2 {
		t.Errorf("writes = %d, want 2", w.calls)
	}
}

func TestProduce_otherWriteError_isReturnedWithoutCreatingAnything(t *testing.T) {
	boom := errors.New("broker unreachable")
	w, a := &fakeWriter{results: []error{boom}}, &fakeAdmin{}

	err := (&KafkaProducer{writer: w, admin: a}).Produce(context.Background(), msg)

	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
	if len(a.created) != 0 || w.calls != 1 {
		t.Errorf("created=%v writes=%d, want none and 1", a.created, w.calls)
	}
}

func TestProduce_creationFails_returnsAClearErrorAndDoesNotRetry(t *testing.T) {
	denied := errors.New("not authorized to create topics")
	w, a := &fakeWriter{results: []error{kafka.UnknownTopicOrPartition}}, &fakeAdmin{err: denied}

	err := (&KafkaProducer{writer: w, admin: a}).Produce(context.Background(), msg)

	if !errors.Is(err, denied) || !strings.Contains(err.Error(), msg.Topic) {
		t.Errorf("err = %v, want it to wrap %v and name the topic", err, denied)
	}
	if w.calls != 1 {
		t.Errorf("writes = %d, want 1 (no retry when creation failed)", w.calls)
	}
}

func TestProduce_stillUnknownAfterCreation_returnsTheErrorForTheNextRelayTick(t *testing.T) {
	// Broker metadata can lag a moment behind a fresh topic; the relay polls again and succeeds.
	w, a := &fakeWriter{results: []error{kafka.UnknownTopicOrPartition}}, &fakeAdmin{}

	err := (&KafkaProducer{writer: w, admin: a}).Produce(context.Background(), msg)

	if !errors.Is(err, kafka.UnknownTopicOrPartition) {
		t.Errorf("err = %v, want UnknownTopicOrPartition", err)
	}
	if w.calls != 2 || len(a.created) != 1 {
		t.Errorf("writes=%d created=%d, want 2 and 1", w.calls, len(a.created))
	}
}
