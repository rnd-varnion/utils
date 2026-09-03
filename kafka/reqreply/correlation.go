package reqreply

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/rnd-varnion/utils/logger"
)

// CorrelationRegistry manages correlation ID to reply channel mapping
type CorrelationRegistry struct {
	mu            sync.RWMutex
	pending       map[string]chan *ReplyMessage
	timeout       time.Duration
	cleanupTicker *time.Ticker
	done          chan struct{}
}

// ReplyMessage represents a reply message received from Kafka
type ReplyMessage struct {
	CorrelationID string
	Payload       []byte
	Error         error
}

// NewCorrelationRegistry creates a new correlation registry
func NewCorrelationRegistry(timeout time.Duration) *CorrelationRegistry {
	if timeout == 0 {
		timeout = 10 * time.Second // default timeout
	}

	registry := &CorrelationRegistry{
		pending: make(map[string]chan *ReplyMessage),
		timeout: timeout,
		done:    make(chan struct{}),
	}

	// Start cleanup routine
	registry.startCleanup()

	logger.Log.Infof("[INFO] Correlation registry created with timeout: %v\n", timeout)
	return registry
}

// GenerateCorrelationID generates a random 16-byte correlation ID
func (r *CorrelationRegistry) Register(correlationID string) chan *ReplyMessage {
	replyChan := make(chan *ReplyMessage, 1)

	r.mu.Lock()
	r.pending[correlationID] = replyChan
	r.mu.Unlock()

	logger.Log.Debugf("[DEBUG] Registered correlation ID: %s\n", correlationID)
	return replyChan
}

// GenerateCorrelationID generates a random 16-byte correlation ID
func (r *CorrelationRegistry) GenerateCorrelationID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID if random fails
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	return hex.EncodeToString(b)
}

// Deliver delivers a reply message to the appropriate waiting channel
func (r *CorrelationRegistry) Deliver(correlationID string, payload []byte, err error) bool {
	r.mu.RLock()
	replyChan, exists := r.pending[correlationID]
	r.mu.RUnlock()

	if !exists {
		logger.Log.Warnf("[WARN] No pending request for correlation ID: %s\n", correlationID)
		return false
	}

	// Send reply (non-blocking to avoid deadlock)
	select {
	case replyChan <- &ReplyMessage{
		CorrelationID: correlationID,
		Payload:       payload,
		Error:         err,
	}:
		logger.Log.Debugf("[DEBUG] Delivered reply for correlation ID: %s\n", correlationID)
		return true
	default:
		logger.Log.Warnf("[WARN] Reply channel full for correlation ID: %s\n", correlationID)
		return false
	}
}

// Unregister removes a correlation ID from the registry
func (r *CorrelationRegistry) Unregister(correlationID string) {
	r.mu.Lock()
	if replyChan, exists := r.pending[correlationID]; exists {
		close(replyChan)
		delete(r.pending, correlationID)
		logger.Log.Debugf("[DEBUG] Unregistered correlation ID: %s\n", correlationID)
	}
	r.mu.Unlock()
}

// WaitForReply waits for a reply with the given correlation ID and timeout
func (r *CorrelationRegistry) WaitForReply(ctx context.Context, correlationID string, timeout time.Duration) (*ReplyMessage, error) {
	r.mu.RLock()
	replyChan, exists := r.pending[correlationID]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("correlation ID %s not found in registry", correlationID)
	}

	// Create context with timeout if not already set
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Wait for reply or timeout
	select {
	case reply := <-replyChan:
		r.Unregister(correlationID)
		return reply, nil
	case <-ctx.Done():
		r.Unregister(correlationID)
		return nil, ctx.Err()
	}
}

// startCleanup starts a background routine to clean up stale entries
func (r *CorrelationRegistry) startCleanup() {
	r.cleanupTicker = time.NewTicker(30 * time.Second)

	go func() {
		for {
			select {
			case <-r.cleanupTicker.C:
				r.cleanup()
			case <-r.done:
				r.cleanupTicker.Stop()
				return
			}
		}
	}()

	logger.Log.Info("[INFO] Started correlation registry cleanup routine")
}

// cleanup removes stale entries from the registry
func (r *CorrelationRegistry) cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Note: This is a basic cleanup. In production, you might want to track
	// timestamps and remove entries older than the timeout
	if len(r.pending) > 0 {
		logger.Log.Debugf("[DEBUG] Cleanup: %d pending requests in registry\n", len(r.pending))
	}
}

// Close closes the registry and cleans up resources
func (r *CorrelationRegistry) Close() {
	close(r.done)

	r.mu.Lock()
	defer r.mu.Unlock()

	// Close all pending channels
	for correlationID, replyChan := range r.pending {
		close(replyChan)
		delete(r.pending, correlationID)
		logger.Log.Debugf("[DEBUG] Closed channel for correlation ID: %s\n", correlationID)
	}

	logger.Log.Info("[INFO] Correlation registry closed")
}

// GetPendingCount returns the number of pending requests
func (r *CorrelationRegistry) GetPendingCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.pending)
}
