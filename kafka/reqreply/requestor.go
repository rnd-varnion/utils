package reqreply

import (
	"context"
	"fmt"
	"time"

	"github.com/rnd-varnion/utils/logger"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Requestor handles sending requests and waiting for replies
type Requestor struct {
	client              *Client
	registry            *CorrelationRegistry
	requestTopic        string
	replyTopic          string
	consumerGroup       string
	timeout             time.Duration
	replyConsumer       *kgo.Client
	stopReplyConsumer   chan struct{}
}

// NewRequestor creates a new requestor instance
func NewRequestor(client *Client, registry *CorrelationRegistry, requestTopic, replyTopic, consumerGroup string) *Requestor {
	if client == nil {
		return nil
	}

	timeout := 10 * time.Second // default timeout

	return &Requestor{
		client:            client,
		registry:          registry,
		requestTopic:      requestTopic,
		replyTopic:        replyTopic,
		consumerGroup:     consumerGroup,
		timeout:          timeout,
		stopReplyConsumer: make(chan struct{}),
	}
}

// SetTimeout sets the request timeout
func (r *Requestor) SetTimeout(timeout time.Duration) {
	r.timeout = timeout
}

// Start initializes the reply consumer
func (r *Requestor) Start() error {
	if r.client == nil {
		return fmt.Errorf("client not initialized")
	}

	logger.Log.Infof("[INFO] Starting requestor - Request Topic: %s, Reply Topic: %s\n", r.requestTopic, r.replyTopic)

	// Create consumer specifically for replies
	consumer := r.client.GetConsumer()
	if consumer == nil {
		return fmt.Errorf("consumer not initialized")
	}

	// Assign consumer to reply topic using consumer group
	consumer.AddConsumeTopics(r.replyTopic)

	r.replyConsumer = consumer

	// Start reply consumer goroutine
	go r.consumeReplies()

	logger.Log.Info("[INFO] Requestor started successfully")
	return nil
}

// SendRequest sends a request and waits for a reply
func (r *Requestor) SendRequest(ctx context.Context, payload []byte) ([]byte, error) {
	if r.registry == nil {
		return nil, fmt.Errorf("correlation registry not initialized")
	}

	// Generate correlation ID
	correlationID := r.registry.GenerateCorrelationID()

	// Register the request with correlation ID
	_ = r.registry.Register(correlationID)
	defer r.registry.Unregister(correlationID)

	logger.Log.Infof("[INFO] Sending request with correlation ID: %s to topic: %s\n", correlationID, r.requestTopic)

	// Send request message
	producer := r.client.GetProducer()
	if producer == nil {
		return nil, fmt.Errorf("producer not initialized")
	}

	// Create record with correlation ID as header
	record := &kgo.Record{
		Topic: r.requestTopic,
		Key:   []byte(correlationID),
		Value: payload,
		Headers: []kgo.RecordHeader{
			{Key: "correlation_id", Value: []byte(correlationID)},
		},
	}

	// Produce message asynchronously
	producer.Produce(ctx, record, func(record *kgo.Record, err error) {
		if err != nil {
			logger.Log.Errorf("[ERROR] Failed to send request: %v\n", err)
			// Deliver error to the waiting channel
			r.registry.Deliver(correlationID, nil, err)
		} else {
			logger.Log.Infof("[INFO] Request sent successfully with correlation ID: %s\n", correlationID)
		}
	})

	// Wait for reply with timeout
	reply, err := r.registry.WaitForReply(ctx, correlationID, r.timeout)
	if err != nil {
		return nil, fmt.Errorf("request failed or timed out: %w", err)
	}

	if reply.Error != nil {
		return nil, fmt.Errorf("reply error: %w", reply.Error)
	}

	logger.Log.Infof("[INFO] Received reply for correlation ID: %s\n", correlationID)
	return reply.Payload, nil
}

// consumeReplies continuously consumes replies from the reply topic
func (r *Requestor) consumeReplies() {
	logger.Log.Info("[INFO] Starting reply consumer goroutine")

	for {
		select {
		case <-r.stopReplyConsumer:
			logger.Log.Info("[INFO] Stopping reply consumer")
			return
		default:
			if r.replyConsumer == nil {
				time.Sleep(1 * time.Second)
				continue
			}

			// Poll for records
			fetches := r.replyConsumer.PollRecords(context.Background(), 100)
			if len(fetches) == 0 {
				continue
			}

			// Process each record
			fetches.EachRecord(func(record *kgo.Record) {
				// Extract correlation ID from headers
				var correlationID string
				for _, header := range record.Headers {
					if header.Key == "correlation_id" {
						correlationID = string(header.Value)
						break
					}
				}

				if correlationID == "" {
					logger.Log.Warn("[WARN] Received reply without correlation ID")
					return
				}

				logger.Log.Debugf("[DEBUG] Received reply for correlation ID: %s\n", correlationID)

				// Deliver reply to waiting request
				r.registry.Deliver(correlationID, record.Value, nil)
			})

			// Commit offsets
			if err := r.replyConsumer.CommitUncommittedOffsets(context.Background()); err != nil {
				logger.Log.Errorf("[ERROR] Failed to commit offsets: %v\n", err)
			}
		}
	}
}

// Stop stops the requestor and cleans up resources
func (r *Requestor) Stop() error {
	logger.Log.Info("[INFO] Stopping requestor")

	close(r.stopReplyConsumer)

	logger.Log.Info("[INFO] Requestor stopped")
	return nil
}

// GetRequestTopic returns the request topic
func (r *Requestor) GetRequestTopic() string {
	return r.requestTopic
}

// GetReplyTopic returns the reply topic
func (r *Requestor) GetReplyTopic() string {
	return r.replyTopic
}

// GetPendingCount returns the number of pending requests
func (r *Requestor) GetPendingCount() int {
	if r.registry != nil {
		return r.registry.GetPendingCount()
	}
	return 0
}
