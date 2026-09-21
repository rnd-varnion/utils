package event

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/rnd-varnion/utils/kafka/reqreply"
	"github.com/rnd-varnion/utils/logger"
	"github.com/twmb/franz-go/pkg/kgo"
)

// EventHandler defines the function signature for processing an incoming event
type EventHandler func(ctx context.Context, evt *Event) error

// Subscriber consumes events from Kafka topics and dispatches them to registered handlers by event type
type Subscriber struct {
	client         *reqreply.Client
	topics         []string
	consumerGroup  string
	handlers       map[string]EventHandler
	defaultHandler EventHandler
	mu             sync.RWMutex
	stopChan       chan struct{}
	running        bool
}

// NewSubscriber creates a new event subscriber instance
func NewSubscriber(client *reqreply.Client, topics []string, consumerGroup string) *Subscriber {
	if client == nil {
		return nil
	}

	return &Subscriber{
		client:        client,
		topics:        topics,
		consumerGroup: consumerGroup,
		handlers:      make(map[string]EventHandler),
		stopChan:      make(chan struct{}),
	}
}

// RegisterHandler binds an event type string to an EventHandler implementation
func (s *Subscriber) RegisterHandler(eventType string, handler EventHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[eventType] = handler
	logger.Log.Infof("[INFO] Registered handler for event type: %s\n", eventType)
}

// RegisterDefaultHandler registers a fallback handler for unhandled event types
func (s *Subscriber) RegisterDefaultHandler(handler EventHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.defaultHandler = handler
	logger.Log.Info("[INFO] Registered default event handler")
}

// Start begins consuming events from subscribed topics
func (s *Subscriber) Start() error {
	if s.client == nil {
		return fmt.Errorf("kafka client not initialized")
	}

	consumer := s.client.GetConsumer()
	if consumer == nil {
		return fmt.Errorf("consumer not initialized")
	}

	logger.Log.Infof("[INFO] Starting event subscriber for topics: %v with consumer group: %s\n", s.topics, s.consumerGroup)

	consumer.AddConsumeTopics(s.topics...)
	s.running = true

	go s.consumeLoop(consumer)

	logger.Log.Info("[INFO] Event subscriber started successfully")
	return nil
}

func (s *Subscriber) consumeLoop(consumer *kgo.Client) {
	for s.running {
		select {
		case <-s.stopChan:
			logger.Log.Info("[INFO] Event subscriber consume loop stopping")
			return
		default:
			fetches := consumer.PollRecords(context.Background(), 100)
			if len(fetches) == 0 {
				continue
			}

			fetches.EachRecord(func(record *kgo.Record) {
				var evt Event
				if err := json.Unmarshal(record.Value, &evt); err != nil {
					logger.Log.Errorf("[ERROR] Failed to unmarshal event record from topic %s: %v\n", record.Topic, err)
					return
				}

				s.mu.RLock()
				handler, exists := s.handlers[evt.Type]
				if !exists {
					handler = s.defaultHandler
				}
				s.mu.RUnlock()

				if handler == nil {
					logger.Log.Warnf("[WARN] No handler registered for event type: %s (ID: %s)\n", evt.Type, evt.ID)
					return
				}

				// Execute handler asynchronously
				go func(e Event, h EventHandler) {
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()

					if err := h(ctx, &e); err != nil {
						logger.Log.Errorf("[ERROR] Handler returned error for event %s (type: %s): %v\n", e.ID, e.Type, err)
					} else {
						logger.Log.Debugf("[DEBUG] Handled event %s (type: %s) successfully\n", e.ID, e.Type)
					}
				}(evt, handler)
			})

			if err := consumer.CommitUncommittedOffsets(context.Background()); err != nil {
				logger.Log.Errorf("[ERROR] Failed to commit offsets: %v\n", err)
			}
		}
	}
}

// Stop gracefully stops the subscriber consume loop
func (s *Subscriber) Stop() error {
	logger.Log.Info("[INFO] Stopping event subscriber")
	s.running = false
	close(s.stopChan)
	return nil
}

// IsRunning returns whether the subscriber loop is active
func (s *Subscriber) IsRunning() bool {
	return s.running
}
