package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rnd-varnion/utils/kafka/common"
	"github.com/rnd-varnion/utils/kafka/reqreply"
	"github.com/rnd-varnion/utils/logger"
)

// CustomHandler demonstrates custom business logic for request processing
func CustomHandler(correlationID string, payload []byte) ([]byte, error) {
	// Example business logic: uppercase the input and add prefix
	processedPayload := "pong: " + strings.ToUpper(string(payload))
	logger.Log.Infof("[INFO] Processing request %s: %s -> %s\n", correlationID, string(payload), processedPayload)

	return []byte(processedPayload), nil
}

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

	// Define topics
	requestTopic := "data-request"
	replyTopic := "data-reply"
	requestorGroup := "advanced-requestor"
	responderGroup := "advanced-responder"

	// Create topic manager and ensure topics exist
	topicManager := reqreply.NewTopicManager(client.GetProducer(), config.Brokers)
	if topicManager != nil {
		ctx := context.Background()
		if err := topicManager.CreateTopics(ctx, []string{requestTopic, replyTopic}); err != nil {
			logger.Log.Warnf("[WARN] Topic creation failed: %v\n", err)
		}
		defer topicManager.Close()
	}

	// Create correlation registry for requestor
	registry := reqreply.NewCorrelationRegistry(15 * time.Second)
	defer registry.Close()

	fmt.Println("=== Advanced Request-Reply Example ===")

	// Start Responder (Service B)
	responder := reqreply.NewResponder(client, requestTopic, replyTopic, responderGroup)
	responder.SetHandler(CustomHandler)

	if err := responder.Start(); err != nil {
		logger.Log.Errorf("[ERROR] Failed to start responder: %v\n", err)
		os.Exit(1)
	}
	defer responder.Stop()

	// Give responder time to start
	time.Sleep(2 * time.Second)

	// Start Requestor (Service A)
	requestor := reqreply.NewRequestor(client, registry, requestTopic, replyTopic, requestorGroup)
	if err := requestor.Start(); err != nil {
		logger.Log.Errorf("[ERROR] Failed to start requestor: %v\n", err)
		os.Exit(1)
	}
	defer requestor.Stop()

	// Give requestor time to be ready
	time.Sleep(2 * time.Second)

	// Send multiple requests concurrently
	fmt.Println("Sending multiple requests concurrently...")

	requests := []string{
		"hello world",
		"kafka request-reply",
		"advanced usage",
		"test message",
	}

	// Send requests concurrently
	for _, reqText := range requests {
		go func(payload string) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			reply, err := requestor.SendRequest(ctx, []byte(payload))
			if err != nil {
				logger.Log.Errorf("[ERROR] Request '%s' failed: %v\n", payload, err)
				return
			}

			fmt.Printf("Request: '%s' -> Reply: '%s'\n", payload, string(reply))
		}(reqText)
	}

	// Wait for all requests to complete
	time.Sleep(5 * time.Second)

	// Show some stats
	fmt.Printf("\n=== Statistics ===\n")
	fmt.Printf("Pending requests: %d\n", requestor.GetPendingCount())
	fmt.Printf("Responder consuming: %v\n", responder.IsConsuming())

	// Send a request that will timeout
	fmt.Println("\nTesting timeout with invalid correlation ID...")
	_, err = requestor.SendRequest(context.Background(), []byte("timeout test"))
	if err != nil {
		fmt.Printf("Expected timeout/error: %v\n", err)
	}

	fmt.Println("\n=== Advanced Example Complete ===")

	// Wait for cleanup
	time.Sleep(2 * time.Second)
}
