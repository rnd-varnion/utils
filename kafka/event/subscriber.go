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
	dlqConfig      *DeadLetterQueueConfig
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

// WithDeadLetterQueue configures automatic in-memory retries and DLQ forwarding on final failure.
// If configured, failed messages will be retried up to maxRetries times with retryDelay backoff.
// If all retries fail, the event is wrapped in a DeadLetterEnvelope and published to dlqTopic.
// Only after DLQ dispatch succeeds will the Kafka offset be committed.
func (s *Subscriber) WithDeadLetterQueue(
	dlqPublisher *Publisher,
	dlqTopic string,
	maxRetries int,
	retryDelay time.Duration,
) *Subscriber {
	if maxRetries <= 0 {
		maxRetries = 3
	}
	if retryDelay <= 0 {
		retryDelay = 2 * time.Second
	}

	s.dlqConfig = &DeadLetterQueueConfig{
		Publisher:  dlqPublisher,
		Topic:      dlqTopic,
		MaxRetries: maxRetries,
		RetryDelay: retryDelay,
	}
	return s
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

				// Merge native Kafka headers into evt.Headers (jika belum ada di JSON payload)
				if len(record.Headers) > 0 {
					if evt.Headers == nil {
						evt.Headers = make(map[string]string)
					}
					for _, h := range record.Headers {
						if _, exists := evt.Headers[h.Key]; !exists {
							evt.Headers[h.Key] = string(h.Value)
						}
					}
				}

				s.mu.RLock()
				handler, exists := s.handlers[evt.Type]
				if !exists {
					handler = s.defaultHandler
				}
				s.mu.RUnlock()

				if isRetry, ok := evt.Headers["is_retry"]; ok && isRetry == "true" {
					if failedService, exists := evt.Headers["failed_service"]; exists && failedService != "" {
						if failedService != s.consumerGroup {
							logger.Log.Debugf("[DEBUG] Skipping retry event %s (%s) targeted for service '%s' (current: '%s')\n",
								evt.ID, evt.Type, failedService, s.consumerGroup)
							return
						}
						logger.Log.Infof("[INFO] Processing targeted retry event %s (%s) for consumer group '%s'\n",
							evt.ID, evt.Type, s.consumerGroup)
					}
				}

				if handler == nil {
					logger.Log.Warnf("[WARN] No handler registered for event type: %s (ID: %s)\n", evt.Type, evt.ID)
					return
				}

				wg.Add(1)
				go func(e Event, h EventHandler, srcTopic string) {
					defer wg.Done()
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()

					// 1. Eksekusi pertama
					err := h(ctx, &e)
					if err == nil {
						logger.Log.Debugf("[DEBUG] Handled event %s (type: %s) successfully\n", e.ID, e.Type)
						return
					}

					logger.Log.Errorf("[ERROR] Handler returned error for event %s (type: %s): %v\n", e.ID, e.Type, err)

					// 2. Jika DLQ tidak dikonfigurasi, gunakan perilaku lama (backward compatibility)
					if s.dlqConfig == nil || s.dlqConfig.Publisher == nil || s.dlqConfig.Topic == "" {
						return
					}

					// 3. Retry Loop dengan Backoff
					var lastErr = err
					for attempt := 1; attempt <= s.dlqConfig.MaxRetries; attempt++ {
						select {
						case <-ctx.Done():
							logger.Log.Warnf("[WARN] Retry context expired for event %s: %v\n", e.ID, ctx.Err())
							return
						case <-time.After(s.dlqConfig.RetryDelay):
						}

						if retryErr := h(ctx, &e); retryErr == nil {
							logger.Log.Infof("[INFO] Retry attempt %d/%d succeeded for event %s\n", attempt, s.dlqConfig.MaxRetries, e.ID)
							return // Sukses setelah retry!
						} else {
							lastErr = retryErr
							logger.Log.Warnf("[WARN] Retry attempt %d/%d failed for event %s: %v\n", attempt, s.dlqConfig.MaxRetries, e.ID, retryErr)
						}
					}

					// 4. Seluruh percobaan gagal -> Bungkus ke DLQ Envelope & Publish
					dlqPayload := &DeadLetterEnvelope{
						OriginalEventID:   e.ID,
						OriginalEventType: e.Type,
						SourceTopic:       srcTopic,
						ConsumerGroup:     s.consumerGroup,
						ErrorMessage:      lastErr.Error(),
						RetryCount:        s.dlqConfig.MaxRetries,
						FailedAt:          time.Now().UTC().Format(time.RFC3339),
						Payload:           json.RawMessage(e.Payload),
						Headers:           e.Headers,
					}

					payloadBytes, marshalErr := json.Marshal(dlqPayload)
					if marshalErr != nil {
						logger.Log.Errorf("[CRITICAL] Failed to marshal DLQ envelope for event %s: %v\n", e.ID, marshalErr)
						return
					}

					dlqEvt := NewEvent(e.Type+".failed", s.consumerGroup, payloadBytes)
					dlqEvt.WithHeader("tenant_id", "varnion-nexus")
					dlqEvt.WithHeader("failed_service", s.consumerGroup)

					// Gunakan context independen baru dengan PublishSync khusus untuk DLQ
					// agar tidak terpengaruh oleh cancel() handler dan memastikan pesan sampai sebelum offset di-commit
					dlqCtx, dlqCancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer dlqCancel()

					if pubErr := s.dlqConfig.Publisher.PublishSync(dlqCtx, s.dlqConfig.Topic, dlqEvt); pubErr != nil {
						logger.Log.Errorf("[CRITICAL] Failed to publish event %s to DLQ %s: %v\n", e.ID, s.dlqConfig.Topic, pubErr)
					} else {
						logger.Log.Warnf("[DLQ] Successfully forwarded failed event %s to DLQ topic: %s\n", e.ID, s.dlqConfig.Topic)
					}
				}(evt, handler, record.Topic)
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
