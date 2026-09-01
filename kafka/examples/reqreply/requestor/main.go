package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/rnd-varnion/utils/kafka/common"
	"github.com/rnd-varnion/utils/kafka/reqreply"
)

// RequestPayload defines outgoing request schema
type RequestPayload struct {
	Command string `json:"command"`
	Data    string `json:"data"`
}

// ResponsePayload defines incoming response schema
type ResponsePayload struct {
	CorrelationID string `json:"correlationId"`
	Status        string `json:"status"`
	Result        string `json:"result,omitempty"`
	Error         string `json:"error,omitempty"`
	ProcessedAt   string `json:"processed_at"`
}

const (
	RequestTopic = "service.orders.request"
	ReplyTopic   = "service.orders.reply"
	GroupID      = "order-requestor-client"
)

func main() {
	log.Println("[REQUESTOR CLIENT] Starting Order Client...")

	// 1. Load config
	config := common.LoadConfigFromEnv()

	// 2. Create Kafka client
	client, err := reqreply.NewClient(config)
	if err != nil {
		log.Fatalf("[REQUESTOR CLIENT] Failed to create client: %v", err)
	}
	defer client.Close()

	// 3. Create correlation registry
	registry := reqreply.NewCorrelationRegistry(10 * time.Second)
	defer registry.Close()

	// 4. Create and start Requestor
	requestor := reqreply.NewRequestor(client, registry, RequestTopic, ReplyTopic, GroupID)
	if err := requestor.Start(); err != nil {
		log.Fatalf("[REQUESTOR CLIENT] Failed to start requestor: %v", err)
	}
	defer requestor.Stop()

	// Give subscription a moment to connect
	time.Sleep(1 * time.Second)

	// Send test requests
	sendOrder(requestor, "", "CREATE_ORDER", "Laptop Pro 16-inch")
	sendOrder(requestor, "custom-trace-id-12345", "CANCEL_ORDER", "ORD-99201")
	sendOrder(requestor, "", "UNKNOWN_CMD", "Invalid test payload")

	log.Println("[REQUESTOR CLIENT] All requests processed.")
}

func sendOrder(requestor *reqreply.Requestor, customCorrelationID, command, data string) {
	req := RequestPayload{
		Command: command,
		Data:    data,
	}

	payload, err := json.Marshal(req)
	if err != nil {
		log.Printf("Failed to marshal request: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var replyBytes []byte
	if customCorrelationID != "" {
		log.Printf("[REQUESTOR CLIENT] Sending command '%s' with Custom Correlation ID '%s'...", command, customCorrelationID)
		replyBytes, err = requestor.SendRequestWithID(ctx, customCorrelationID, payload)
	} else {
		log.Printf("[REQUESTOR CLIENT] Sending command '%s' with data '%s'...", command, data)
		replyBytes, err = requestor.SendRequest(ctx, payload)
	}
	if err != nil {
		log.Printf("[REQUESTOR CLIENT] ❌ Error waiting for reply: %v", err)
		return
	}

	var resp ResponsePayload
	if err := json.Unmarshal(replyBytes, &resp); err != nil {
		log.Printf("[REQUESTOR CLIENT] Raw reply bytes: %s", string(replyBytes))
		return
	}

	if resp.Status == "SUCCESS" {
		log.Printf("[REQUESTOR CLIENT] ✅ Response SUCCESS [CorrelationID: %s]: %s", resp.CorrelationID, resp.Result)
	} else {
		log.Printf("[REQUESTOR CLIENT] ⚠️ Response ERROR [CorrelationID: %s]: %s", resp.CorrelationID, resp.Error)
	}
}
