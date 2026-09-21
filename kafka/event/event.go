package event

import (
	"time"

	"github.com/google/uuid"
)

// Event represents a standard event message envelope
type Event struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	Source    string            `json:"source"`
	Timestamp time.Time         `json:"timestamp"`
	Payload   []byte            `json:"payload"`
	Headers   map[string]string `json:"headers,omitempty"`
}

// NewEvent creates a new Event instance with an automatically generated UUID and UTC timestamp
func NewEvent(eventType, source string, payload []byte) *Event {
	return &Event{
		ID:        uuid.NewString(),
		Type:      eventType,
		Source:    source,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
		Headers:   make(map[string]string),
	}
}

// WithHeader adds a custom key-value header to the event
func (e *Event) WithHeader(key, value string) *Event {
	if e.Headers == nil {
		e.Headers = make(map[string]string)
	}
	e.Headers[key] = value
	return e
}
