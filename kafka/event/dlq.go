package event

import (
	"encoding/json"
	"time"
)

// DeadLetterQueueConfig holds the configuration for automatic retry and DLQ forwarding
type DeadLetterQueueConfig struct {
	Publisher  *Publisher
	Topic      string
	MaxRetries int
	RetryDelay time.Duration
}

// DeadLetterEnvelope defines the standardized payload format sent to the DLQ topic
type DeadLetterEnvelope struct {
	OriginalEventID   string            `json:"original_event_id"`
	OriginalEventType string            `json:"original_event_type"`
	SourceTopic       string            `json:"source_topic"`
	ConsumerGroup     string            `json:"consumer_group"`
	ErrorMessage      string            `json:"error_message"`
	RetryCount        int               `json:"retry_count"`
	FailedAt          string            `json:"failed_at"`
	Payload           json.RawMessage   `json:"payload"`
	Headers           map[string]string `json:"headers,omitempty"`
}
