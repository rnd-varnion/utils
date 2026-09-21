package main

import (
	"context"
	"fmt"
	"time"

	"github.com/rnd-varnion/utils/kafka/common"
	"github.com/rnd-varnion/utils/kafka/event"
	"github.com/rnd-varnion/utils/kafka/reqreply"
)

func main() {
	// 1. Load Kafka configuration from environment variables
	config := common.LoadConfigFromEnv()

	// 2. Initialize Kafka client
	client, err := reqreply.NewClient(config)
	if err != nil {
		panic(err)
	}
	defer client.Close()

	topic := "user-events"
	consumerGroup := "user-events-group"

	// 3. Create Subscriber and register Event Handlers
	subscriber := event.NewSubscriber(client, []string{topic}, consumerGroup)

	// Handler for "user.created"
	subscriber.RegisterHandler("user.created", func(ctx context.Context, evt *event.Event) error {
		fmt.Printf(" [Consumer] Received 'user.created' event (ID: %s, Source: %s): %s\n", evt.ID, evt.Source, string(evt.Payload))
		return nil
	})

	// Handler for "user.deleted"
	subscriber.RegisterHandler("user.deleted", func(ctx context.Context, evt *event.Event) error {
		fmt.Printf(" [Consumer] Received 'user.deleted' event (ID: %s): %s\n", evt.ID, string(evt.Payload))
		return nil
	})

	// Start subscriber
	if err := subscriber.Start(); err != nil {
		panic(err)
	}
	defer subscriber.Stop()

	// 4. Create Publisher and send events
	publisher := event.NewPublisher(client)

	// Publish "user.created" event
	evt1 := event.NewEvent("user.created", "user-service", []byte(`{"user_id": 101, "email": "alice@varnion.com"}`))
	evt1.WithHeader("tenant_id", "varnion-prod")

	if err := publisher.Publish(context.Background(), topic, evt1); err != nil {
		fmt.Printf("Failed to publish event: %v\n", err)
	} else {
		fmt.Printf(" [Publisher] Published 'user.created' event (ID: %s)\n", evt1.ID)
	}

	// Publish "user.deleted" event
	evt2 := event.NewEvent("user.deleted", "admin-service", []byte(`{"user_id": 101, "reason": "requested_by_user"}`))
	if err := publisher.Publish(context.Background(), topic, evt2); err != nil {
		fmt.Printf("Failed to publish event: %v\n", err)
	} else {
		fmt.Printf(" [Publisher] Published 'user.deleted' event (ID: %s)\n", evt2.ID)
	}

	// Wait briefly for consumer processing
	time.Sleep(2 * time.Second)
}
