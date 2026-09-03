package reqreply

import (
	"context"
	"fmt"
	"time"

	"github.com/rnd-varnion/utils/logger"
	"github.com/twmb/franz-go/pkg/kgo"
)

// RequestHandler defines the interface for handling incoming requests
type RequestHandler func(correlationID string, payload []byte) ([]byte, error)

// Responder handles consuming requests and sending replies
type Responder struct {
	client            *Client
	requestTopic      string
	replyTopic        string
	consumerGroup     string
	handler           RequestHandler
	consumer          *kgo.Client
	stopConsuming     chan struct{}
	consuming         bool
}

// NewResponder creates a new responder instance
func NewResponder(client *Client, requestTopic, replyTopic, consumerGroup string) *Responder {
	if client == nil {
		return nil
	}

	return &Responder{
		client:        client,
		requestTopic:  requestTopic,
		replyTopic:    replyTopic,
		consumerGroup: consumerGroup,
		stopConsuming: make(chan struct{}),
		consuming:     false,
	}
}

// SetHandler sets the request handler function
func (r *Responder) SetHandler(handler RequestHandler) {
	r.handler = handler
}

// Start begins consuming requests and processing them
func (r *Responder) Start() error {
	if r.client == nil {
		return fmt.Errorf("client not initialized")
	}

	if r.handler == nil {
		return fmt.Errorf("request handler not set. Use SetHandler() first")
	}

	logger.Log.Infof("[INFO] Starting responder - Request Topic: %s, Reply Topic: %s\n", r.requestTopic, r.replyTopic)

	// Get consumer from client
	consumer := r.client.GetConsumer()
	if consumer == nil {
		return fmt.Errorf("consumer not initialized")
	}

	// Assign consumer to request topic
	consumer.AddConsumeTopics(r.requestTopic)

	r.consumer = consumer
	r.consuming = true

	// Start request consumer goroutine
	go r.consumeRequests()

	logger.Log.Info("[INFO] Responder started successfully")
	return nil
}

// consumeRequests continuously consumes requests from the request topic
func (r *Responder) consumeRequests() {
	logger.Log.Info("[INFO] Starting request consumer goroutine")

	for r.consuming {
		select {
		case <-r.stopConsuming:
			logger.Log.Info("[INFO] Stopping request consumer")
			return
		default:
			if r.consumer == nil {
				time.Sleep(1 * time.Second)
				continue
			}

			// Poll for records
			fetches := r.consumer.PollRecords(context.Background(), 100)
			if len(fetches) == 0 {
				continue
			}

			// Process each record
			fetches.EachRecord(func(record *kgo.Record) {
				// Extract correlation ID from headers
				var correlationID string
				for _, header := range record.Headers {
					if header.Key == "correlationId" || header.Key == "correlation_id" {
						correlationID = string(header.Value)
						break
					}
				}

				if correlationID == "" {
					logger.Log.Warn("[WARN] Received request without correlation ID")
					return
				}

				logger.Log.Infof("[INFO] Processing request with correlation ID: %s\n", correlationID)

				// Process request asynchronously
				go r.handleRequest(correlationID, record.Value)
			})

			// Commit offsets
			if err := r.consumer.CommitUncommittedOffsets(context.Background()); err != nil {
				logger.Log.Errorf("[ERROR] Failed to commit offsets: %v\n", err)
			}
		}
	}
}

// handleRequest processes a single request and sends a reply
func (r *Responder) handleRequest(correlationID string, payload []byte) {
	// Call the handler to process the request
	response, err := r.handler(correlationID, payload)
	if err != nil {
		logger.Log.Errorf("[ERROR] Handler returned error for correlation ID %s: %v\n", correlationID, err)

		// Send error response
		r.sendReply(correlationID, nil, err)
		return
	}

	logger.Log.Infof("[INFO] Handler processed request for correlation ID: %s\n", correlationID)

	// Send successful response
	r.sendReply(correlationID, response, nil)
}

// sendReply sends a reply message to the reply topic
func (r *Responder) sendReply(correlationID string, payload []byte, err error) {
	producer := r.client.GetProducer()
	if producer == nil {
		logger.Log.Errorf("[ERROR] Producer not initialized for correlation ID: %s\n", correlationID)
		return
	}

	// Create reply record with correlation ID as header
	var replyValue []byte
	if err != nil {
		// If there's an error, send error message as payload
		replyValue = []byte("ERROR: " + err.Error())
	} else {
		replyValue = payload
	}

	record := &kgo.Record{
		Topic: r.replyTopic,
		Key:   []byte(correlationID),
		Value: replyValue,
		Headers: []kgo.RecordHeader{
			{Key: "correlationId", Value: []byte(correlationID)},
		},
	}

	// Produce reply message
	producer.Produce(context.Background(), record, func(record *kgo.Record, err error) {
		if err != nil {
			logger.Log.Errorf("[ERROR] Failed to send reply for correlation ID %s: %v\n", correlationID, err)
		} else {
			logger.Log.Infof("[INFO] Reply sent successfully for correlation ID: %s\n", correlationID)
		}
	})
}

// Stop stops the responder and cleans up resources
func (r *Responder) Stop() error {
	logger.Log.Info("[INFO] Stopping responder")

	r.consuming = false
	close(r.stopConsuming)

	logger.Log.Info("[INFO] Responder stopped")
	return nil
}

// GetRequestTopic returns the request topic
func (r *Responder) GetRequestTopic() string {
	return r.requestTopic
}

// GetReplyTopic returns the reply topic
func (r *Responder) GetReplyTopic() string {
	return r.replyTopic
}

// IsConsuming returns whether the responder is currently consuming
func (r *Responder) IsConsuming() bool {
	return r.consuming
}
