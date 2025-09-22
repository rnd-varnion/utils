package example

import (
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
