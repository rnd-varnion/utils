// Package kafka provides a per-service Kafka bootstrap that bundles the
// reqreply primitives (Client, CorrelationRegistry, TopicManager) into a
// single reusable wrapper, so services don't repeat the same wiring code.
package kafka

import (
	"context"
	"fmt"
	"time"

	"github.com/rnd-varnion/utils/kafka/common"
	"github.com/rnd-varnion/utils/kafka/reqreply"
)

type Kafka struct {
	Client       *reqreply.Client
	Registry     *reqreply.CorrelationRegistry
	TopicManager *reqreply.TopicManager
}

func Init(config *common.Config) (*Kafka, error) {
	client, err := reqreply.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kafka client: %w", err)
	}

	registry := reqreply.NewCorrelationRegistry(10 * time.Second)

	return &Kafka{
		Client:       client,
		Registry:     registry,
		TopicManager: reqreply.NewTopicManager(client.GetProducer(), config.Brokers),
	}, nil
}

// EnsureTopics checks that the given topics exist on the broker and creates
// any that are missing, as a programmatic alternative to broker-side
// auto topic creation.
func (k *Kafka) EnsureTopics(topics ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, topic := range topics {
		exists, err := k.TopicManager.TopicExists(ctx, topic)
		if err != nil {
			return fmt.Errorf("failed to check kafka topic %s: %w", topic, err)
		}

		if exists {
			continue
		}

		if err := k.TopicManager.CreateTopic(ctx, topic); err != nil {
			return fmt.Errorf("failed to create kafka topic %s: %w", topic, err)
		}
	}

	return nil
}

func (k *Kafka) Close() {
	if k.Registry != nil {
		k.Registry.Close()
	}

	if k.TopicManager != nil {
		_ = k.TopicManager.Close()
	}

	if k.Client != nil {
		k.Client.Close()
	}
}
