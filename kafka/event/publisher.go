package event

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rnd-varnion/utils/kafka/reqreply"
	"github.com/rnd-varnion/utils/logger"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Publisher emits events to Kafka topics asynchronously or synchronously
type Publisher struct {
	client *reqreply.Client
}

// NewPublisher creates a new event publisher
func NewPublisher(client *reqreply.Client) *Publisher {
	if client == nil {
		return nil
	}
	return &Publisher{client: client}
}

// Publish sends an event asynchronously to a specified Kafka topic (fire-and-forget)
func (p *Publisher) Publish(ctx context.Context, topic string, evt *Event) error {
	if p.client == nil {
		return fmt.Errorf("kafka client not initialized")
	}
	if evt == nil {
		return fmt.Errorf("event cannot be nil")
	}

	producer := p.client.GetProducer()
	if producer == nil {
		return fmt.Errorf("producer not initialized")
	}

	data, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	headers := []kgo.RecordHeader{
		{Key: "event_id", Value: []byte(evt.ID)},
		{Key: "event_type", Value: []byte(evt.Type)},
		{Key: "event_source", Value: []byte(evt.Source)},
	}

	for k, v := range evt.Headers {
		headers = append(headers, kgo.RecordHeader{Key: k, Value: []byte(v)})
	}

	record := &kgo.Record{
		Topic:   topic,
		Key:     []byte(evt.ID),
		Value:   data,
		Headers: headers,
	}

	producer.Produce(ctx, record, func(rec *kgo.Record, err error) {
		if err != nil {
			logger.Log.Errorf("[ERROR] Failed to publish event %s (type: %s) to topic %s: %v\n", evt.ID, evt.Type, topic, err)
		} else {
			logger.Log.Debugf("[DEBUG] Successfully published event %s (type: %s) to topic %s\n", evt.ID, evt.Type, topic)
		}
	})

	return nil
}

// PublishSync sends an event synchronously to a specified Kafka topic and waits for broker ACK
func (p *Publisher) PublishSync(ctx context.Context, topic string, evt *Event) error {
	if p.client == nil {
		return fmt.Errorf("kafka client not initialized")
	}
	if evt == nil {
		return fmt.Errorf("event cannot be nil")
	}

	producer := p.client.GetProducer()
	if producer == nil {
		return fmt.Errorf("producer not initialized")
	}

	data, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	headers := []kgo.RecordHeader{
		{Key: "event_id", Value: []byte(evt.ID)},
		{Key: "event_type", Value: []byte(evt.Type)},
		{Key: "event_source", Value: []byte(evt.Source)},
	}

	for k, v := range evt.Headers {
		headers = append(headers, kgo.RecordHeader{Key: k, Value: []byte(v)})
	}

	record := &kgo.Record{
		Topic:   topic,
		Key:     []byte(evt.ID),
		Value:   data,
		Headers: headers,
	}

	results := producer.ProduceSync(ctx, record)
	if err := results.FirstErr(); err != nil {
		return fmt.Errorf("failed to sync publish event %s: %w", evt.ID, err)
	}

	logger.Log.Infof("[INFO] Synchronously published event %s (type: %s) to topic %s\n", evt.ID, evt.Type, topic)
	return nil
}
