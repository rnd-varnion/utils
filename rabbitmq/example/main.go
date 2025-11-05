package example

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rnd-varnion/utils/rabbitmq"
)

type Data struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Param struct {
		Type string `json:"type"`
	} `json:"param"`
}

func main() {
	// Initiate engine
	rabbit := rabbitmq.New(rabbitmq.NewConfig{
		Username: "guest",
		Password: "guest",
		Host:     "localhost:5672",
	})

	// Start consuming from vhost "/" and queue "data"
	msgs, err := rabbit.Consume(rabbitmq.ConsumeConfig{
		Vhost: "/",
		Queue: "data",
	})
	if err != nil {
		panic(err)
	}

	// Process messages
	go func() {
		for msg := range msgs {
			fmt.Println("Received raw:", msg)
			var r Data
			if err := json.Unmarshal([]byte(msg), &r); err != nil {
				fmt.Println("Parse error:", err)
				continue
			}

			// Do something with the data
			fmt.Printf("Processed data ID %s\n", r.ID)
		}
	}()

	// Give consumer time to boot (optional, but recommended)
	time.Sleep(2 * time.Second)

	// -----------------------------------------------------------------
	// Publish a test message
	r := Data{
		ID:    "abc123",
		Title: "test",
	}
	r.Param.Type = "common"

	b, _ := json.Marshal(r)

	// Publish message to vhost "/" and queue "data"
	err = rabbit.Publish(rabbitmq.PublishConfig{
		Vhost:   "/",
		Queue:   "data",
		Message: string(b),
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("Published:", string(b))

	// Publish a delay message to vhost "/" and queue "data"
	err = rabbit.PublishWithDelay(rabbitmq.PublishConfig{
		Vhost:   "/",
		Queue:   "data",
		Message: string(b),
	}, 5*time.Second)
	if err != nil {
		panic(err)
	}
	fmt.Println("Published:", string(b))

	// Keep alive
	select {}
}

// ExampleAutoAck Auto-acknowledge (original behavior)
func ExampleAutoAck() {
	rabbit := rabbitmq.New(rabbitmq.NewConfig{
		Username: "guest",
		Password: "guest",
		Host:     "localhost:5672",
	})

	msgs, err := rabbit.Consume(rabbitmq.ConsumeConfig{
		Vhost: "/",
		Queue: "data",
	})
	if err != nil {
		panic(err)
	}

	go func() {
		for msg := range msgs {
			fmt.Println("Received raw:", msg)
			var r Data
			if err := json.Unmarshal([]byte(msg), &r); err != nil {
				fmt.Println("Parse error:", err)
				continue
			}
			fmt.Printf("Processed data ID %s\n", r.ID)
		}
	}()

	time.Sleep(2 * time.Second)

	// Publish test message
	r := Data{ID: "abc123", Title: "test"}
	r.Param.Type = "common"
	b, _ := json.Marshal(r)

	err = rabbit.Publish(rabbitmq.PublishConfig{
		Vhost:   "/",
		Queue:   "data",
		Message: string(b),
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("Published:", string(b))

	select {}
}

// ExampleManualAck Manual acknowledgment
func ExampleManualAck() {
	rabbit := rabbitmq.New(rabbitmq.NewConfig{
		Username: "guest",
		Password: "guest",
		Host:     "localhost:5672",
	})

	// Consume with manual ack, prefetch 5 messages
	msgs, err := rabbit.ConsumeAck(rabbitmq.ConsumeAckConfig{
		Vhost:         "/",
		Queue:         "data-manual",
		PrefetchCount: 5,
	})
	if err != nil {
		panic(err)
	}

	go func() {
		for msg := range msgs {
			fmt.Printf("Received message (tag %d): %s\n", msg.DeliveryTag, msg.Body)

			var r Data
			if err := json.Unmarshal([]byte(msg.Body), &r); err != nil {
				fmt.Println("Parse error:", err)
				// Reject message without requeue on parse error
				if err := msg.Reject(); err != nil {
					fmt.Println("Reject error:", err)
				}
				continue
			}

			// Process the data
			if r.Param.Type == "critical" {
				// If processing fails, requeue for retry
				if err := processData(r); err != nil {
					fmt.Println("Processing failed, requeuing:", err)
					msg.Requeue()
					continue
				}
			}

			// Successfully processed, acknowledge
			if err := msg.Ack(); err != nil {
				fmt.Println("Ack error:", err)
			} else {
				fmt.Printf("Successfully processed and acked: %s\n", r.ID)
			}
		}
	}()

	time.Sleep(2 * time.Second)

	// Publish test messages
	for i := 0; i < 10; i++ {
		r := Data{
			ID:    fmt.Sprintf("manual-%d", i),
			Title: fmt.Sprintf("test-%d", i),
		}
		if i%3 == 0 {
			r.Param.Type = "critical"
		} else {
			r.Param.Type = "normal"
		}

		b, _ := json.Marshal(r)
		err = rabbit.Publish(rabbitmq.PublishConfig{
			Vhost:   "/",
			Queue:   "data-manual",
			Message: string(b),
		})
		if err != nil {
			panic(err)
		}
	}

	fmt.Println("Published 10 messages with manual ack")

	select {}
}

// ExampleGracefulShutdown Graceful shutdown
func ExampleGracefulShutdown() {
	rabbit := rabbitmq.New(rabbitmq.NewConfig{
		Username: "guest",
		Password: "guest",
		Host:     "localhost:5672",
	})

	msgs, err := rabbit.ConsumeAck(rabbitmq.ConsumeAckConfig{
		Vhost:         "/",
		Queue:         "data-shutdown",
		PrefetchCount: 3,
	})
	if err != nil {
		panic(err)
	}

	done := make(chan bool)

	go func() {
		for msg := range msgs {
			var r Data
			json.Unmarshal([]byte(msg.Body), &r)
			fmt.Printf("Processing: %s\n", r.ID)

			// Simulate processing time
			time.Sleep(2 * time.Second)

			msg.Ack()
			fmt.Printf("Completed: %s\n", r.ID)
		}
		done <- true
	}()

	time.Sleep(5 * time.Second)

	// Graceful shutdown
	fmt.Println("Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := rabbit.Shutdown(); err != nil {
		fmt.Println("Shutdown error:", err)
	}

	select {
	case <-done:
		fmt.Println("All messages processed")
	case <-ctx.Done():
		fmt.Println("Shutdown timeout")
	}
}

// Helper functions
func processData(r Data) error {
	// Simulate processing that might fail
	if r.ID == "manual-3" {
		return fmt.Errorf("simulated processing error")
	}
	time.Sleep(100 * time.Millisecond)
	return nil
}
