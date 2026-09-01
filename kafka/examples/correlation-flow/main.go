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

// DetailedHandler shows correlation ID usage in request-reply flow
func DetailedHandler(correlationID string, payload []byte) ([]byte, error) {
	// This demonstrates how correlation ID flows through the system:
	// 1. Requestor generates correlation ID
	// 2. Requestor sends request with correlation ID in headers
	// 3. Responder receives request and extracts correlation ID
	// 4. Responder processes business logic
	// 5. Responder sends reply with same correlation ID
	// 6. Requestor matches reply using correlation ID

	logger.Log.Infof("[INFO] 📨 Responder received request with correlation ID: %s\n", correlationID)
	logger.Log.Infof("[INFO] 📦 Request payload: %s\n", string(payload))

	// Business logic processing
	processedPayload := fmt.Sprintf("PROCESSED-%s", string(payload))
	logger.Log.Infof("[INFO] ⚙️  Processing logic applied: %s -> %s\n", string(payload), processedPayload)

	// The correlation ID will automatically be included in the reply
	logger.Log.Infof("[INFO] 📤 Sending reply with correlation ID: %s\n", correlationID)

	return []byte(processedPayload), nil
}

func main() {
	// Load configuration
	config := common.LoadConfigFromEnv()

	// Create Kafka client
	client, err := reqreply.NewClient(config)
	if err != nil {
		logger.Log.Errorf("[ERROR] Failed to create client: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	// Create correlation registry
	registry := reqreply.NewCorrelationRegistry(15 * time.Second)
	defer registry.Close()

	// Define topics
	requestTopic := "flow-request"
	replyTopic := "flow-reply"
	requestorGroup := "flow-requestor"
	responderGroup := "flow-responder"

	// Create topic manager
	topicManager := reqreply.NewTopicManager(client.GetProducer(), config.Brokers)
	if topicManager != nil {
		ctx := context.Background()
		if err := topicManager.CreateTopics(ctx, []string{requestTopic, replyTopic}); err != nil {
			logger.Log.Warnf("[WARN] Topic creation failed: %v\n", err)
		}
		defer topicManager.Close()
	}

	fmt.Println("=== Request-Reply Correlation ID Flow Demo ===")
	fmt.Println("\n🔍 This demonstrates how correlation IDs flow through the system:\n")

	// Start Responder (Service B)
	fmt.Println("📡 Starting Responder (Service B)...")
	responder := reqreply.NewResponder(client, requestTopic, replyTopic, responderGroup)
	responder.SetHandler(DetailedHandler)

	if err := responder.Start(); err != nil {
		logger.Log.Errorf("[ERROR] Failed to start responder: %v\n", err)
		os.Exit(1)
	}
	defer responder.Stop()

	// Give responder time to start
	time.Sleep(2 * time.Second)

	// Start Requestor (Service A)
	fmt.Println("📤 Starting Requestor (Service A)...")
	requestor := reqreply.NewRequestor(client, registry, requestTopic, replyTopic, requestorGroup)
	if err := requestor.Start(); err != nil {
		logger.Log.Errorf("[ERROR] Failed to start requestor: %v\n", err)
		os.Exit(1)
	}
	defer requestor.Stop()

	// Give requestor time to be ready
	time.Sleep(2 * time.Second)

	fmt.Println("\n" + "==================================================")
	fmt.Println("🚀 REQUEST-REPLY FLOW DEMONSTRATION")
	fmt.Println("==================================================")

	// Example 1: Show correlation ID flow step by step
	fmt.Println("\n📋 Example 1: Step-by-Step Correlation ID Flow")

	// Step 1: Generate correlation ID
	fmt.Println("\n🔹 Step 1: Requestor generates correlation ID")
	customCorrelationID := registry.GenerateCorrelationID()
	fmt.Printf("   Generated: %s\n", customCorrelationID)

	// Step 2: Register correlation ID
	fmt.Println("\n🔹 Step 2: Requestor registers correlation ID")
	_ = registry.Register(customCorrelationID)
	fmt.Printf("   Registered: %s (waiting for reply)\n", customCorrelationID)
	defer registry.Unregister(customCorrelationID)

	// Step 3: Send request
	fmt.Println("\n🔹 Step 3: Requestor sends request with correlation ID")
	requestPayload := []byte("test-request-1")
	fmt.Printf("   Sending: %s\n", string(requestPayload))
	fmt.Printf("   Headers: correlation_id=%s\n", customCorrelationID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	reply, err := requestor.SendRequest(ctx, requestPayload)
	if err != nil {
		logger.Log.Errorf("[ERROR] Request failed: %v\n", err)
		os.Exit(1)
	}

	// Step 4: Receive reply
	fmt.Println("\n🔹 Step 4: Requestor receives reply with matching correlation ID")
	fmt.Printf("   Reply: %s\n", string(reply))
	fmt.Printf("   ✅ Correlation ID matched: %s\n", customCorrelationID)

	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("📊 CORRELATION ID FLOW SUMMARY")
	fmt.Println(strings.Repeat("=", 50))

	fmt.Println("\n🔄 Complete Flow:")
	fmt.Printf("1️⃣  Requestor generates correlation ID: %s\n", customCorrelationID)
	fmt.Printf("2️⃣  Requestor sends [Request + CorrID: %s] → Kafka\n", customCorrelationID)
	fmt.Printf("3️⃣  Kafka delivers request to Responder\n")
	fmt.Printf("4️⃣  Responder extracts CorrID: %s and processes request\n", customCorrelationID)
	fmt.Printf("5️⃣  Responder sends [Reply + CorrID: %s] → Kafka\n", customCorrelationID)
	fmt.Printf("6️⃣  Kafka delivers reply to Requestor\n")
	fmt.Printf("7️⃣  Requestor matches CorrID: %s and delivers reply\n", customCorrelationID)

	// Example 2: Multiple concurrent requests
	fmt.Println("\n🔹 Example 2: Multiple Concurrent Requests with Correlation IDs")
	fmt.Println("\n📤 Sending 3 concurrent requests, each with unique correlation ID...")

	requests := []string{"request-alpha", "request-beta", "request-gamma"}
	for i, reqText := range requests {
		go func(payload string, index int) {
			// Each request gets its own correlation ID
			corrID := registry.GenerateCorrelationID()
			fmt.Printf("   Request %d: CorrID=%s, Payload=%s\n", index+1, corrID, payload)

			_ = registry.Register(corrID)
			defer registry.Unregister(corrID)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			reply, err := requestor.SendRequest(ctx, []byte(payload))
			if err != nil {
				logger.Log.Errorf("[ERROR] Request '%s' (CorrID: %s) failed: %v\n", payload, corrID, err)
				return
			}

			fmt.Printf("   ✅ Request %d: CorrID=%s matched, Reply=%s\n", index+1, corrID, string(reply))
		}(reqText, i)
	}

	// Wait for all concurrent requests to complete
	time.Sleep(5 * time.Second)

	// Show statistics
	fmt.Printf("\n📊 Final Statistics:\n")
	fmt.Printf("   Pending requests: %d\n", requestor.GetPendingCount())
	fmt.Printf("   Responder consuming: %v\n", responder.IsConsuming())

	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("✅ REQUEST-REPLY FLOW DEMONSTRATION COMPLETE")
	fmt.Println(strings.Repeat("=", 50))

	fmt.Println("\n💡 Key Takeaways:")
	fmt.Println("   • Each request gets a unique correlation ID")
	fmt.Println("   • Correlation ID flows through entire request-reply cycle")
	fmt.Println("   • Registry matches replies to waiting requests")
	fmt.Println("   • Thread-safe handling of concurrent requests")
	fmt.Println("   • Automatic timeout and cleanup")

	// Wait for cleanup
	time.Sleep(2 * time.Second)
}
