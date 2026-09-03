package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rnd-varnion/utils/kafka/common"
	"github.com/rnd-varnion/utils/kafka/reqreply"
)

// RequestPayload defines incoming request schema
type RequestPayload struct {
	Command string `json:"command"`
	Data    string `json:"data"`
}

// ResponsePayload defines outgoing response schema
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
	GroupID      = "order-responder-service"
)

func main() {
	log.Println("[RESPONDER SERVICE] Starting Order Processor Service...")

	// 1. Load config from environment (or default localhost:9092)
	config := common.LoadConfigFromEnv()

	// 2. Create Kafka client
	client, err := reqreply.NewClient(config)
	if err != nil {
		log.Fatalf("[RESPONDER SERVICE] Failed to create client: %v", err)
	}
	defer client.Close()

	// 3. Create Responder
	responder := reqreply.NewResponder(client, RequestTopic, ReplyTopic, GroupID)

	// 4. Set Business Logic Handler
	responder.SetHandler(func(correlationID string, payload []byte) ([]byte, error) {
		log.Printf("[RESPONDER SERVICE] Processing request ID: %s | Raw payload: %s", correlationID, string(payload))

		var req RequestPayload
		if err := json.Unmarshal(payload, &req); err != nil {
			res := ResponsePayload{
				CorrelationID: correlationID,
				Status:        "FAILED",
				Error:         fmt.Sprintf("Invalid JSON: %v", err),
				ProcessedAt:   time.Now().Format(time.RFC3339),
			}
			return json.Marshal(res)
		}

		// Perform business logic based on command
		switch req.Command {
		case "CREATE_ORDER":
			res := ResponsePayload{
				CorrelationID: correlationID,
				Status:        "SUCCESS",
				Result:        fmt.Sprintf("Order created for item: %s", req.Data),
				ProcessedAt:   time.Now().Format(time.RFC3339),
			}
			return json.Marshal(res)

		case "CANCEL_ORDER":
			res := ResponsePayload{
				CorrelationID: correlationID,
				Status:        "SUCCESS",
				Result:        fmt.Sprintf("Order %s cancelled", req.Data),
				ProcessedAt:   time.Now().Format(time.RFC3339),
			}
			return json.Marshal(res)

		default:
			res := ResponsePayload{
				CorrelationID: correlationID,
				Status:        "ERROR",
				Error:         fmt.Sprintf("Unsupported command: %s", req.Command),
				ProcessedAt:   time.Now().Format(time.RFC3339),
			}
			return json.Marshal(res)
		}
	})

	// 5. Start consuming requests
	if err := responder.Start(); err != nil {
		log.Fatalf("[RESPONDER SERVICE] Error starting responder: %v", err)
	}
	defer responder.Stop()

	log.Printf("[RESPONDER SERVICE] Listening on topic '%s', replying to '%s'...", RequestTopic, ReplyTopic)

	// Keep service running until OS signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("[RESPONDER SERVICE] Shutting down gracefully...")
}
