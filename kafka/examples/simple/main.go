package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rnd-varnion/utils/kafka/common"
	"github.com/rnd-varnion/utils/kafka/reqreply"
	"github.com/rnd-varnion/utils/logger"
)

func main() {
	// Load environment variables
	config := common.LoadConfigFromEnv()

	// Create Kafka client
	client, err := reqreply.NewClient(config)
	if err != nil {
		logger.Log.Errorf("[ERROR] Failed to create client: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	// Create correlation registry
	registry := reqreply.NewCorrelationRegistry(10 * time.Second)
	defer registry.Close()

	// Define topics
	requestTopic := "data-request"
	replyTopic := "data-reply"
	consumerGroup := "simple-requestor"

	// Create topic manager and ensure topics exist
	topicManager := reqreply.NewTopicManager(client.GetProducer(), config.Brokers)
	if topicManager != nil {
		// Create topics if they don't exist
		ctx := context.Background()
		if err := topicManager.CreateTopics(ctx, []string{requestTopic, replyTopic}); err != nil {
			logger.Log.Warnf("[WARN] Topic creation failed: %v\n", err)
		}
		defer topicManager.Close()
	}

	// Simple example: Send a request and wait for reply
	fmt.Println("=== Simple Request-Reply Example ===")

	// Create requestor
	requestor := reqreply.NewRequestor(client, registry, requestTopic, replyTopic, consumerGroup)
	if err := requestor.Start(); err != nil {
		logger.Log.Errorf("[ERROR] Failed to start requestor: %v\n", err)
		os.Exit(1)
	}
	defer requestor.Stop()

	// Give time for consumer to be ready
	time.Sleep(2 * time.Second)

	// Send a request
	requestPayload := []byte("hello from requestor")
	fmt.Printf("Sending request: %s\n", string(requestPayload))

	replyPayload, err := requestor.SendRequest(context.Background(), requestPayload)
	if err != nil {
		logger.Log.Errorf("[ERROR] Request failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Received reply: %s\n", string(replyPayload))
	fmt.Println("=== Example Complete ===")

	// Wait a bit to ensure all messages are processed
	time.Sleep(2 * time.Second)
}
