package event

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/rnd-varnion/utils/kafka/common"
	"github.com/rnd-varnion/utils/kafka/reqreply"
	"github.com/rnd-varnion/utils/logger"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

// EventHandler defines the function signature for processing an incoming event
type EventHandler func(ctx context.Context, evt *Event) error

// Subscriber consumes events from Kafka topics using a consumer group and dispatches them to registered handlers
type Subscriber struct {
	client         *kgo.Client
	topics         []string
	consumerGroup  string
	handlers       map[string]EventHandler
	defaultHandler EventHandler
	mu             sync.RWMutex
	stopChan       chan struct{}
	running        bool
}

// NewSubscriber creates a new event subscriber instance using reqreply.Client configuration
func NewSubscriber(client *reqreply.Client, topics []string, consumerGroup string) *Subscriber {
	if client == nil {
		return nil
	}

	sub, err := NewSubscriberWithConfig(client.GetConfig(), topics, consumerGroup)
	if err != nil {
		logger.Log.Errorf("[ERROR] Failed to create event subscriber: %v\n", err)
		return nil
	}
	return sub
}

// NewSubscriberWithConfig creates a new event subscriber instance with explicit common.Config
func NewSubscriberWithConfig(cfg *common.Config, topics []string, consumerGroup string) (*Subscriber, error) {
	if cfg == nil {
		cfg = common.LoadConfigFromEnv()
	}

	client, err := newConsumerGroupClient(cfg, consumerGroup, topics)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer group client: %w", err)
	}

	return &Subscriber{
		client:        client,
		topics:        topics,
		consumerGroup: consumerGroup,
		handlers:      make(map[string]EventHandler),
		stopChan:      make(chan struct{}),
	}, nil
}

func newConsumerGroupClient(cfg *common.Config, group string, topics []string) (*kgo.Client, error) {
	clientID := cfg.ClientID
	if clientID == "" {
		clientID = "event-subscriber"
	}

	opts := []kgo.Opt{
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ClientID(clientID + "-" + group),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topics...),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	}

	if cfg.Username != "" && cfg.Password != "" {
		saslMech := scram.Auth{
			User: cfg.Username,
			Pass: cfg.Password,
		}.AsSha512Mechanism()
		opts = append(opts, kgo.SASL(saslMech))
	}

	if cfg.CACertPath != "" {
		caCert, err := os.ReadFile(cfg.CACertPath)
		if err == nil {
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(caCert)
			tlsConfig := &tls.Config{
				RootCAs:    caCertPool,
				MinVersion: tls.VersionTLS12,
			}
			opts = append(opts, kgo.DialTLSConfig(tlsConfig))
		}
	}

	return kgo.NewClient(opts...)
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
		return fmt.Errorf("kafka consumer client not initialized")
	}

	logger.Log.Infof("[INFO] Starting event subscriber for topics: %v with consumer group: %s\n", s.topics, s.consumerGroup)

	s.running = true
	go s.consumeLoop()

	logger.Log.Info("[INFO] Event subscriber started successfully")
	return nil
}

func (s *Subscriber) consumeLoop() {
	for s.running {
		select {
		case <-s.stopChan:
			logger.Log.Info("[INFO] Event subscriber consume loop stopping")
			return
		default:
			fetches := s.client.PollRecords(context.Background(), 100)
			if fetches.IsClientClosed() {
				return
			}
			if len(fetches) == 0 {
				continue
			}

			var wg sync.WaitGroup

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

				wg.Add(1)
				go func(e Event, h EventHandler) {
					defer wg.Done()
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()

					if err := h(ctx, &e); err != nil {
						logger.Log.Errorf("[ERROR] Handler returned error for event %s (type: %s): %v\n", e.ID, e.Type, err)
					} else {
						logger.Log.Debugf("[DEBUG] Handled event %s (type: %s) successfully\n", e.ID, e.Type)
					}
				}(evt, handler)
			})

			wg.Wait()

			if err := s.client.CommitUncommittedOffsets(context.Background()); err != nil {
				logger.Log.Errorf("[ERROR] Failed to commit offsets: %v\n", err)
			}
		}
	}
}

// Stop gracefully stops the subscriber consume loop
func (s *Subscriber) Stop() error {
	logger.Log.Info("[INFO] Stopping event subscriber")
	s.running = false
	if s.stopChan != nil {
		close(s.stopChan)
	}
	if s.client != nil {
		s.client.Close()
	}
	return nil
}

// IsRunning returns whether the subscriber loop is active
func (s *Subscriber) IsRunning() bool {
	return s.running
}
