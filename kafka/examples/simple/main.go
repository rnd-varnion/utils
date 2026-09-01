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

	// Simple example: Send a request and wait for reply with correlation ID management
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

	// Example 1: Simple request with automatic correlation ID (recommended)
	fmt.Println("\n--- Example 1: Automatic Correlation ID ---")
	requestPayload := []byte("hello from requestor")
	fmt.Printf("Sending request: %s\n", string(requestPayload))

	replyPayload, err := requestor.SendRequest(context.Background(), requestPayload)
	if err != nil {
		logger.Log.Errorf("[ERROR] Request failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Received reply: %s\n", string(replyPayload))

	// Example 2: Manual correlation ID management (advanced usage)
	fmt.Println("\n--- Example 2: Manual Correlation ID Management ---")

	// Generate and register correlation ID manually
	customCorrelationID := registry.GenerateCorrelationID()
	fmt.Printf("Generated custom correlation ID: %s\n", customCorrelationID)

	// Register the correlation ID
	_ = registry.Register(customCorrelationID)
	defer registry.Unregister(customCorrelationID)

	// In a real scenario, you would use this correlation ID when sending the request
	// and then wait for the reply on the replyChan
	fmt.Printf("Correlation ID %s registered, ready to send request\n", customCorrelationID)

	// For demonstration, we'll use the automatic method which handles all the plumbing
	fmt.Println("Note: Automatic method recommended for production use")

	fmt.Println("\n=== Example Complete ===")

	// Wait a bit to ensure all messages are processed
	time.Sleep(2 * time.Second)
}
