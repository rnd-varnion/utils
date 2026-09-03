package reqreply

import (
	"context"
	"errors"
	"fmt"

	"github.com/rnd-varnion/utils/logger"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
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
	//
	// Note: kadm includes requested-but-missing topics in the details map with
	// Err set (e.g. ErrUnknownTopicOrPartition), so key presence alone is not
	// sufficient — a topic only exists if its Err is nil.
	if detail, exists := topicsDetails[topic]; exists && detail.Err == nil {
		logger.Log.Infof("[INFO] Topic '%s' already exists\n", topic)
		return nil
	}

	// Create the topic using simplified API
	logger.Log.Infof("[INFO] Creating topic '%s' with %d partitions and replication factor %d\n",
		topic, tm.partitions, tm.replicationFactor)

	// Use CreateTopics with proper parameters
	resp, err := tm.admin.CreateTopics(ctx, tm.partitions, tm.replicationFactor, nil, topic)
	if err != nil {
		return fmt.Errorf("failed to create topic '%s': %w", topic, err)
	}

	// Check the per-topic response for errors (responses are a slice)
	for _, ct := range resp {
		if ct.Err == nil {
			continue
		}
		// A TOPIC_ALREADY_EXISTS error means another process beat us to it —
		// that's fine for an ensure operation.
		if errors.Is(ct.Err, kerr.TopicAlreadyExists) {
			logger.Log.Infof("[INFO] Topic '%s' already exists (idempotent creation)\n", topic)
			return nil
		}
		return fmt.Errorf("failed to create topic '%s': %w", topic, ct.Err)
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

	// Check if the topic exists in the details. kadm includes requested-but-
	// missing topics in the map with Err set, so a topic only exists if its
	// detail entry has a nil Err.
	detail, exists := topicsDetails[topic]
	return exists && detail.Err == nil, nil
}

// Close closes the topic manager and its admin client
func (tm *TopicManager) Close() error {
	if tm.admin != nil {
		tm.admin.Close()
		logger.Log.Info("[INFO] Topic manager closed")
	}
	return nil
}
