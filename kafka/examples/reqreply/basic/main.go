package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/rnd-varnion/utils/kafka/common"
	"github.com/rnd-varnion/utils/kafka/reqreply"
)

// UserRequest represents the payload sent from Requestor to Responder
type UserRequest struct {
	Action string `json:"action"` // e.g. "GET_USER", "UPDATE_STATUS"
	UserID string `json:"user_id"`
}

// UserResponse represents the payload returned from Responder to Requestor
type UserResponse struct {
	Success   bool   `json:"success"`
	UserID    string `json:"user_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Email     string `json:"email,omitempty"`
	Status    string `json:"status,omitempty"`
	ErrorMsg  string `json:"error_msg,omitempty"`
	Timestamp string `json:"timestamp"`
}

const (
	RequestTopic = "user.service.requests"
	ReplyTopic   = "user.service.replies"
	GroupReq     = "user-requestor-group"
	GroupResp    = "user-responder-group"
)

func main() {
	log.Println("==================================================")
	log.Println(" Kafka Request-Reply Pattern Example (All-in-One)")
	log.Println("==================================================")

	// 1. Load configuration from environment (or fallback defaults)
	config := common.LoadConfigFromEnv()

	// 2. Create topic manager and ensure topics exist
	setupClient, err := reqreply.NewClient(config)
	if err != nil {
		log.Fatalf("Failed to create setup client: %v", err)
	}

	topicManager := reqreply.NewTopicManager(setupClient.GetProducer(), config.Brokers)
	ctxSetup, cancelSetup := context.WithTimeout(context.Background(), 10*time.Second)
	if err := topicManager.CreateTopics(ctxSetup, []string{RequestTopic, ReplyTopic}); err != nil {
		log.Printf("Warning: topic setup encountered issue (topics may already exist): %v", err)
	}
	cancelSetup()
	setupClient.Close()

	// -------------------------------------------------------------------------
	// 3. Start Responder Service
	// -------------------------------------------------------------------------
	respClient, err := reqreply.NewClient(config)
	if err != nil {
		log.Fatalf("Failed to create responder client: %v", err)
	}
	defer respClient.Close()

	responder := reqreply.NewResponder(respClient, RequestTopic, ReplyTopic, GroupResp)

	// Define request handler logic
	responder.SetHandler(func(correlationID string, payload []byte) ([]byte, error) {
		log.Printf("[RESPONDER] Received request [CorrelationID: %s], body: %s", correlationID, string(payload))

		var req UserRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			resp := UserResponse{
				Success:   false,
				ErrorMsg:  fmt.Sprintf("Invalid JSON request: %v", err),
				Timestamp: time.Now().Format(time.RFC3339),
			}
			return json.Marshal(resp)
		}

		// Process request based on action
		switch req.Action {
		case "GET_USER":
			if req.UserID == "404" {
				resp := UserResponse{
					Success:   false,
					ErrorMsg:  "User not found",
					Timestamp: time.Now().Format(time.RFC3339),
				}
				return json.Marshal(resp)
			}

			resp := UserResponse{
				Success:   true,
				UserID:    req.UserID,
				Name:      fmt.Sprintf("John Doe (%s)", req.UserID),
				Email:     fmt.Sprintf("user-%s@varnion.com", req.UserID),
				Status:    "ACTIVE",
				Timestamp: time.Now().Format(time.RFC3339),
			}
			return json.Marshal(resp)

		case "SLOW_ACTION":
			// Simulate slow processing (useful to demonstrate client timeout)
			time.Sleep(3 * time.Second)
			resp := UserResponse{
				Success:   true,
				UserID:    req.UserID,
				Status:    "COMPLETED_SLOWLY",
				Timestamp: time.Now().Format(time.RFC3339),
			}
			return json.Marshal(resp)

		default:
			resp := UserResponse{
				Success:   false,
				ErrorMsg:  fmt.Sprintf("Unknown action: %s", req.Action),
				Timestamp: time.Now().Format(time.RFC3339),
			}
			return json.Marshal(resp)
		}
	})

	if err := responder.Start(); err != nil {
		log.Fatalf("Failed to start responder: %v", err)
	}
	defer responder.Stop()

	// Wait briefly for consumer group subscription to take effect
	time.Sleep(1 * time.Second)

	// -------------------------------------------------------------------------
	// 4. Start Requestor Service
	// -------------------------------------------------------------------------
	reqClient, err := reqreply.NewClient(config)
	if err != nil {
		log.Fatalf("Failed to create requestor client: %v", err)
	}
	defer reqClient.Close()

	registry := reqreply.NewCorrelationRegistry(10 * time.Second)
	defer registry.Close()

	requestor := reqreply.NewRequestor(reqClient, registry, RequestTopic, ReplyTopic, GroupReq)
	if err := requestor.Start(); err != nil {
		log.Fatalf("Failed to start requestor: %v", err)
	}
	defer requestor.Stop()

	time.Sleep(1 * time.Second)

	// -------------------------------------------------------------------------
	// DEMO 1: Single Successful Request
	// -------------------------------------------------------------------------
	log.Println("\n--- DEMO 1: Single Successful Request ---")
	sendUserRequest(requestor, "GET_USER", "usr-101", 5*time.Second)

	// -------------------------------------------------------------------------
	// DEMO 2: Error Response (User Not Found)
	// -------------------------------------------------------------------------
	log.Println("\n--- DEMO 2: Error Response (User Not Found) ---")
	sendUserRequest(requestor, "GET_USER", "404", 5*time.Second)

	// -------------------------------------------------------------------------
	// DEMO 3: Client Timeout Handling
	// -------------------------------------------------------------------------
	log.Println("\n--- DEMO 3: Request Timeout (Timeout set to 1s, Handler takes 3s) ---")
	sendUserRequest(requestor, "SLOW_ACTION", "usr-102", 1*time.Second)

	// -------------------------------------------------------------------------
	// DEMO 4: Concurrent Requests
	// -------------------------------------------------------------------------
	log.Println("\n--- DEMO 4: Concurrent Requests ---")
	var wg sync.WaitGroup
	userIDs := []string{"usr-201", "usr-202", "usr-203", "usr-204", "usr-205"}

	for _, id := range userIDs {
		wg.Add(1)
		go func(uid string) {
			defer wg.Done()
			sendUserRequest(requestor, "GET_USER", uid, 5*time.Second)
		}(id)
	}
	wg.Wait()

	// -------------------------------------------------------------------------
	// DEMO 5: Custom Correlation ID (e.g., from HTTP X-Request-ID Header)
	// -------------------------------------------------------------------------
	log.Println("\n--- DEMO 5: Request with Custom Correlation ID ---")
	customCorrelationID := "http-req-trace-998877"
	sendUserRequestWithCustomID(requestor, customCorrelationID, "GET_USER", "usr-300", 5*time.Second)

	log.Println("\n==================================================")
	log.Println(" All demos finished. Press Ctrl+C to exit.")
	log.Println("==================================================")

	// Listen for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sigChan:
		log.Println("Shutting down...")
	case <-time.After(2 * time.Second):
		log.Println("Auto-exiting example.")
	}
}

// Helper function to serialize, send request, and print reply
func sendUserRequest(requestor *reqreply.Requestor, action, userID string, timeout time.Duration) {
	sendUserRequestWithCustomID(requestor, "", action, userID, timeout)
}

// Helper function to send request with an optional custom correlation ID
func sendUserRequestWithCustomID(requestor *reqreply.Requestor, customID, action, userID string, timeout time.Duration) {
	reqBody, _ := json.Marshal(UserRequest{
		Action: action,
		UserID: userID,
	})

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	var replyBytes []byte
	var err error

	if customID != "" {
		log.Printf("[REQUESTOR] Sending request with Custom Correlation ID: %s", customID)
		replyBytes, err = requestor.SendRequestWithID(ctx, customID, reqBody)
	} else {
		replyBytes, err = requestor.SendRequest(ctx, reqBody)
	}
	duration := time.Since(start)

	if err != nil {
		log.Printf("[REQUESTOR] ❌ Request (Action: %s, UserID: %s) failed after %v: %v", action, userID, duration, err)
		return
	}

	var resp UserResponse
	if err := json.Unmarshal(replyBytes, &resp); err != nil {
		log.Printf("[REQUESTOR] Raw Reply: %s (duration: %v)", string(replyBytes), duration)
		return
	}

	if resp.Success {
		log.Printf("[REQUESTOR] ✅ Reply Received (duration: %v): UserID=%s, Name=%s, Email=%s, Status=%s",
			duration, resp.UserID, resp.Name, resp.Email, resp.Status)
	} else {
		log.Printf("[REQUESTOR] ⚠️ Error Reply Received (duration: %v): %s", duration, resp.ErrorMsg)
	}
}
