package reqreply

import (
	"context"
	"fmt"

	"github.com/rnd-varnion/utils/logger"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kadm"
)

// TopicManager handles Kafka topic management operations
type TopicManager struct {
	client    *kgo.Client
	admin     *kadm.Client
	brokers   []string
	partitions int32
	replicationFactor int16
}

// NewTopicManager creates a new topic manager
func NewTopicManager(client *kgo.Client, brokers []string) *TopicManager {
	if client == nil {
		return nil
	}

	// Create admin client with the same client
	adminClient := kadm.NewClient(client)

	return &TopicManager{
		client:    client,
		admin:     adminClient,
		brokers:   brokers,
		partitions: 3,           // default partitions
		replicationFactor: 1,    // default replication factor
	}
}

// SetTopicDefaults sets default values for topic creation
func (tm *TopicManager) SetTopicDefaults(partitions int32, replicationFactor int16) {
	tm.partitions = partitions
	tm.replicationFactor = replicationFactor
}

// CreateTopic creates a topic if it doesn't already exist
func (tm *TopicManager) CreateTopic(ctx context.Context, topic string) error {
	if tm.admin == nil {
		return fmt.Errorf("admin client not initialized")
	}

	logger.Log.Infof("[INFO] Checking if topic '%s' exists\n", topic)

	// Check if topic already exists
	topicsDetails, err := tm.admin.ListTopics(ctx, topic)
	if err != nil {
		return fmt.Errorf("failed to list topics: %w", err)
	}

	// If topic exists, log and return
	if _, exists := topicsDetails[topic]; exists {
		logger.Log.Infof("[INFO] Topic '%s' already exists\n", topic)
		return nil
	}

	// Create the topic using simplified API
	logger.Log.Infof("[INFO] Creating topic '%s' with %d partitions and replication factor %d\n",
		topic, tm.partitions, tm.replicationFactor)

	// Use CreateTopics with proper parameters
	_, err = tm.admin.CreateTopics(ctx, tm.partitions, tm.replicationFactor, nil, topic)
	if err != nil {
		// Check if it's a "topic already exists" error (idempotent)
		if isTopicAlreadyExistsError(err) {
			logger.Log.Infof("[INFO] Topic '%s' already exists (idempotent creation)\n", topic)
			return nil
		}
		return fmt.Errorf("failed to create topic '%s': %w", topic, err)
	}

	logger.Log.Infof("[INFO] Successfully created topic '%s'\n", topic)
	return nil
}

// CreateTopics creates multiple topics if they don't already exist
func (tm *TopicManager) CreateTopics(ctx context.Context, topics []string) error {
	for _, topic := range topics {
		if err := tm.CreateTopic(ctx, topic); err != nil {
			return fmt.Errorf("failed to create topic '%s': %w", topic, err)
		}
	}
	return nil
}

// DeleteTopic deletes a topic (use with caution!)
func (tm *TopicManager) DeleteTopic(ctx context.Context, topic string) error {
	if tm.admin == nil {
		return fmt.Errorf("admin client not initialized")
	}

	logger.Log.Warnf("[WARN] Deleting topic '%s'\n", topic)

	// Use DeleteTopics with proper parameters
	resps, err := tm.admin.DeleteTopics(ctx, topic)
	if err != nil {
		return fmt.Errorf("failed to delete topic '%s': %w", topic, err)
	}

	// Check responses for errors
	for _, resp := range resps {
		if err := resp.Err; err != nil {
			return fmt.Errorf("failed to delete topic '%s': %w", topic, err)
		}
		logger.Log.Infof("[INFO] Successfully deleted topic '%s'\n", topic)
	}

	return nil
}

// ListTopics lists all available topics
func (tm *TopicManager) ListTopics(ctx context.Context) (kadm.TopicDetails, error) {
	if tm.admin == nil {
		return kadm.TopicDetails{}, fmt.Errorf("admin client not initialized")
	}

	topicsDetails, err := tm.admin.ListTopics(ctx)
	if err != nil {
		return kadm.TopicDetails{}, fmt.Errorf("failed to list topics: %w", err)
	}

	return topicsDetails, nil
}

// TopicExists checks if a topic exists
func (tm *TopicManager) TopicExists(ctx context.Context, topic string) (bool, error) {
	if tm.admin == nil {
		return false, fmt.Errorf("admin client not initialized")
	}

	topicsDetails, err := tm.admin.ListTopics(ctx, topic)
	if err != nil {
		return false, fmt.Errorf("failed to list topics: %w", err)
	}

	// Check if the topic exists in the details
	_, exists := topicsDetails[topic]
	return exists, nil
}

// Close closes the topic manager and its admin client
func (tm *TopicManager) Close() error {
	if tm.admin != nil {
		tm.admin.Close()
		logger.Log.Info("[INFO] Topic manager closed")
	}
	return nil
}

// isTopicAlreadyExistsError checks if the error is due to topic already existing
func isTopicAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	// Check for common "topic already exists" error patterns
	errStr := err.Error()
	return contains(errStr, "topic already exists") ||
		contains(errStr, "already exists") ||
		contains(errStr, "Topic:")
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			containsMiddle(s, substr)))
}

// containsMiddle checks if substring exists in the middle of string
func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
