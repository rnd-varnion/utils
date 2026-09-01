# Kafka Request-Reply Library

A production-ready Kafka Request-Reply pattern implementation for Go using `franz-go`. This library simplifies building microservices that communicate via synchronous request-reply patterns over Apache Kafka.

## 🎯 Features

- **Request-Reply Pattern**: Complete correlation ID management for request/response matching
- **Thread-Safe Registry**: Concurrent request handling with automatic cleanup
- **Connection Management**: SASL/TLS authentication support, health checking
- **Topic Management**: Auto-creation and validation via Kafka admin API
- **Handler Injection**: Custom business logic processing for requests
- **Consumer Groups**: Built-in consumer group management for scaling
- **Environment Configuration**: Seamless integration with `.env` files
- **Production Ready**: Comprehensive error handling, timeouts, and graceful shutdown

## 📦 Installation

```bash
go get github.com/rnd-varnion/utils/kafka
```

## ⚙️ Configuration

The library automatically loads configuration from environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `KAFKA_BROKERS` | Comma-separated Kafka broker addresses | `localhost:9092` |
| `KAFKA_CLIENT_ID` | Client identifier for connections | `varnion-kafka-client` |
| `KAFKA_USERNAME` | SASL username (optional) | - |
| `KAFKA_PASSWORD` | SASL password (optional) | - |
| `KAFKA_CA_CERT` | Path to CA certificate (optional) | - |
| `KAFKA_SASL_MECHANISM` | SASL mechanism (`sha256`/`sha512`) | `sha256` |

## 📂 Runnable Examples

Complete, copy-paste-ready code examples can be found in the [`examples/`](./examples) directory:

- 🚀 **[All-in-One Runnable Demo](./examples/reqreply/basic/main.go)** (`go run ./kafka/examples/reqreply/basic`): Single-binary demo showing end-to-end request-reply, timeouts, error handling, and concurrency.
- ⚡ **[Responder Service](./examples/reqreply/responder/main.go)** (`go run ./kafka/examples/reqreply/responder`): Standalone worker microservice handling requests.
- 📡 **[Requestor Client](./examples/reqreply/requestor/main.go)** (`go run ./kafka/examples/reqreply/requestor`): Standalone caller microservice sending requests.
- 📖 **[Examples Guide](./examples/README.md)**: Detailed architecture diagram and step-by-step tutorial.

## 🚀 Quick Start

### Basic Request-Reply Pattern

```go
package main

import (
    "context"
    "fmt"
    "os"
    "time"

    "github.com/rnd-varnion/utils/kafka/common"
    "github.com/rnd-varnion/utils/kafka/reqreply"
)

func main() {
    // 1. Load configuration from environment
    config := common.LoadConfigFromEnv()

    // 2. Create Kafka client
    client, err := reqreply.NewClient(config)
    if err != nil {
        panic(err)
    }
    defer client.Close()

    // 3. Create correlation registry
    registry := reqreply.NewCorrelationRegistry(10 * time.Second)
    defer registry.Close()

    // 4. Create and start requestor
    requestor := reqreply.NewRequestor(
        client, registry,
        "data-request", "data-reply", "my-consumer-group",
    )
    if err := requestor.Start(); err != nil {
        panic(err)
    }
    defer requestor.Stop()

    // 5. Send request and wait for reply
    reply, err := requestor.SendRequest(context.Background(), []byte("hello"))
    if err != nil {
        panic(err)
    }

    fmt.Printf("Reply: %s\n", string(reply))
}
```

### Responder Implementation

```go
package main

import (
    "fmt"
    "strings"

    "github.com/rnd-varnion/utils/kafka/common"
    "github.com/rnd-varnion/utils/kafka/reqreply"
)

// CustomHandler processes incoming requests
func CustomHandler(correlationID string, payload []byte) ([]byte, error) {
    // Your business logic here
    result := "pong: " + strings.ToUpper(string(payload))
    return []byte(result), nil
}

func main() {
    config := common.LoadConfigFromEnv()
    client, _ := reqreply.NewClient(config)
    defer client.Close()

    // Create and start responder
    responder := reqreply.NewResponder(
        client, "data-request", "data-reply", "responder-group",
    )
    responder.SetHandler(CustomHandler)

    if err := responder.Start(); err != nil {
        panic(err)
    }
    defer responder.Stop()

    // Keep running...
    select {}
}
```

### Advanced Usage Patterns

#### 1. Concurrent Request Processing

```go
package main

import (
    "context"
    "fmt"
    "sync"
    "time"

    "github.com/rnd-varnion/utils/kafka/common"
    "github.com/rnd-varnion/utils/kafka/reqreply"
)

func main() {
    config := common.LoadConfigFromEnv()
    client, _ := reqreply.NewClient(config)
    defer client.Close()

    registry := reqreply.NewCorrelationRegistry(15 * time.Second)
    defer registry.Close()

    requestor := reqreply.NewRequestor(
        client, registry, "data-request", "data-reply", "concurrent-group",
    )
    requestor.Start()
    defer requestor.Stop()

    // Send multiple requests concurrently
    var wg sync.WaitGroup
    requests := []string{"request-1", "request-2", "request-3", "request-4"}

    for _, req := range requests {
        wg.Add(1)
        go func(payload string) {
            defer wg.Done()

            ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
            defer cancel()

            reply, err := requestor.SendRequest(ctx, []byte(payload))
            if err != nil {
                fmt.Printf("Request '%s' failed: %v\n", payload, err)
                return
            }

            fmt.Printf("Request '%s' -> Reply: '%s'\n", payload, string(reply))
        }(req)
    }

    wg.Wait()
    fmt.Printf("Completed %d requests\n", len(requests))
}
```

#### 2. Advanced Error Handling & Timeouts

```go
package main

import (
    "context"
    "errors"
    "fmt"
    "time"

    "github.com/rnd-varnion/utils/kafka/common"
    "github.com/rnd-varnion/utils/kafka/reqreply"
)

func main() {
    config := common.LoadConfigFromEnv()
    client, _ := reqreply.NewClient(config)
    defer client.Close()

    registry := reqreply.NewCorrelationRegistry(10 * time.Second)
    requestor := reqreply.NewRequestor(client, registry, "req", "reply", "error-group")
    requestor.Start()
    defer requestor.Stop()

    // Handle different error scenarios
    for i := 0; i < 3; i++ {
        ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
        reply, err := requestor.SendRequest(ctx, []byte(fmt.Sprintf("test-%d", i)))

        cancel()

        if err != nil {
            switch {
            case errors.Is(err, context.DeadlineExceeded):
                fmt.Printf("Request %d: Timeout\n", i)
            case errors.Is(err, context.Canceled):
                fmt.Printf("Request %d: Cancelled\n", i)
            default:
                fmt.Printf("Request %d: Error - %v\n", i, err)
            }
            continue
        }

        fmt.Printf("Request %d: Success - %s\n", i, string(reply))
    }
}
```

#### 3. Custom Business Logic Handlers

```go
package main

import (
    "encoding/json"
    "fmt"

    "github.com/rnd-varnion/utils/kafka/common"
    "github.com/rnd-varnion/utils/kafka/reqreply"
)

// Request represents a structured request
type Request struct {
    Operation string                 `json:"operation"`
    Data       map[string]interface{} `json:"data"`
}

// Response represents a structured response
type Response struct {
    Success bool                   `json:"success"`
    Result  interface{}            `json:"result,omitempty"`
    Error   string                 `json:"error,omitempty"`
}

// AdvancedHandler demonstrates complex business logic
func AdvancedHandler(correlationID string, payload []byte) ([]byte, error) {
    var req Request
    if err := json.Unmarshal(payload, &req); err != nil {
        return nil, fmt.Errorf("invalid request format: %w", err)
    }

    var resp Response

    switch req.Operation {
    case "process":
        // Complex processing logic
        result := processRequest(req.Data)
        resp = Response{Success: true, Result: result}

    case "validate":
        // Validation logic
        if isValid, errs := validateRequest(req.Data); !isValid {
            resp = Response{Success: false, Error: fmt.Sprintf("validation failed: %v", errs)}
        } else {
            resp = Response{Success: true, Result: "valid"}
        }

    default:
        resp = Response{Success: false, Error: "unknown operation"}
    }

    responseBytes, err := json.Marshal(resp)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal response: %w", err)
    }

    return responseBytes, nil
}

func main() {
    config := common.LoadConfigFromEnv()
    client, _ := reqreply.NewClient(config)
    defer client.Close()

    responder := reqreply.NewResponder(client, "complex-req", "complex-reply", "processor-group")
    responder.SetHandler(AdvancedHandler)
    responder.Start()
    defer responder.Stop()

    select {} // Keep running
}
```

#### 4. Monitoring & Statistics

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/rnd-varnion/utils/kafka/common"
    "github.com/rnd-varnion/utils/kafka/reqreply"
)

func main() {
    config := common.LoadConfigFromEnv()
    client, _ := reqreply.NewClient(config)
    defer client.Close()

    registry := reqreply.NewCorrelationRegistry(10*time.Second)
    requestor := reqreply.NewRequestor(client, registry, "req", "reply", "monitor-group")
    requestor.Start()
    defer requestor.Stop()

    // Monitor statistics periodically
    go func() {
        ticker := time.NewTicker(5 * time.Second)
        defer ticker.Stop()

        for range ticker.C {
            pendingRequests := requestor.GetPendingCount()
            fmt.Printf("Pending requests: %d\n", pendingRequests)

            registrySize := registry.GetPendingCount()
            fmt.Printf("Registry size: %d\n", registrySize)
        }
    }()

    // Send requests and monitor
    for i := 0; i < 10; i++ {
        go func(id int) {
            ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
            defer cancel()

            _, err := requestor.SendRequest(ctx, []byte(fmt.Sprintf("monitor-test-%d", id)))
            if err != nil {
                fmt.Printf("Request %d failed: %v\n", id, err)
            } else {
                fmt.Printf("Request %d completed\n", id)
            }
        }(i)

        time.Sleep(500 * time.Millisecond)
    }

    time.Sleep(10 * time.Second)
}
```

## 🏗️ Architecture

```
┌─────────────────┐                    ┌─────────────────┐
│   Service A     │                    │   Service B     │
│  (Requestor)    │                    │  (Responder)    │
├─────────────────┤                    ├─────────────────┤
│                 │  Request + CorrID  │                 │
│  SendRequest()  │ ──────────────────> │  Consume        │
│                 │                    │  Process        │
│  Wait for Reply │<────────────────── │  Reply + CorrID │
│                 │     Reply          │                 │
└─────────────────┘                    └─────────────────┘
         │                                       │
         └─────────── Kafka Topics ─────────────┘
                     data-request
                     data-reply
```

## 📚 API Reference

### Core Components

#### `common.Config`
Configuration management with automatic environment variable loading.

```go
config := common.LoadConfigFromEnv()
```

#### `reqreply.Client`
Main Kafka client with connection management.

```go
client, err := reqreply.NewClient(config)
defer client.Close()

// Check connection health
if err := client.Ping(ctx); err != nil {
    // Handle unhealthy connection
}

// Get producer/consumer
producer := client.GetProducer()
consumer := client.GetConsumer()
```

#### `reqreply.CorrelationRegistry`
Thread-safe correlation ID to reply channel mapping.

```go
registry := reqreply.NewCorrelationRegistry(timeout)
defer registry.Close()

// Register request and get correlation ID
correlationID, replyChan := registry.Register()

// Deliver reply
registry.Deliver(correlationID, payload, nil)

// Wait for reply with timeout
reply, err := registry.WaitForReply(ctx, correlationID, timeout)
```

#### `reqreply.Requestor`
Handles sending requests and waiting for replies.

```go
requestor := reqreply.NewRequestor(
    client, registry,
    "request-topic", "reply-topic", "consumer-group",
)

// Start reply consumer
if err := requestor.Start(); err != nil {
    panic(err)
}
defer requestor.Stop()

// Send request and wait for reply
reply, err := requestor.SendRequest(ctx, payload)

// Set custom timeout
requestor.SetTimeout(15 * time.Second)

// Check pending requests
pending := requestor.GetPendingCount()
```

#### `reqreply.Responder`
Handles consuming requests and sending replies.

```go
responder := reqreply.NewResponder(
    client, "request-topic", "reply-topic", "consumer-group",
)

// Set custom handler
responder.SetHandler(func(correlationID string, payload []byte) ([]byte, error) {
    // Your business logic
    return result, nil
})

// Start consuming requests
if err := responder.Start(); err != nil {
    panic(err)
}
defer responder.Stop()

// Check status
isConsuming := responder.IsConsuming()
```

#### `reqreply.TopicManager`
Kafka topic management utilities.

```go
topicManager := reqreply.NewTopicManager(client.GetProducer(), config.Brokers)
defer topicManager.Close()

// Create topics (idempotent)
if err := topicManager.CreateTopics(ctx, []string{"topic1", "topic2"}); err != nil {
    // Handle error
}

// Check if topic exists
exists, err := topicManager.TopicExists(ctx, "my-topic")

// List all topics
topics, err := topicManager.ListTopics(ctx)

// Delete topic (use caution!)
if err := topicManager.DeleteTopic(ctx, "old-topic"); err != nil {
    // Handle error
}
```

## 🎯 Use Cases

- **Microservices Communication**: Synchronous communication between services
- **API Gateways**: Request aggregation from multiple backend services  
- **Event-Driven Architecture**: Request-response pattern over Kafka
- **Data Processing**: On-demand data processing with responses
- **Command Patterns**: Command execution with status reporting

## 📝 Examples

Complete working examples are available in the `examples/` directory:

### Simple Example
Basic request-reply pattern demonstration.
```bash
cd examples/simple
go run main.go
```

### Advanced Example  
Concurrent requests, custom handlers, statistics, and error handling.
```bash
cd examples/advanced
go run main.go
```

## 🔧 Environment Setup

Create a `.env` file in your project root:

```env
KAFKA_BROKERS=localhost:9092
KAFKA_CLIENT_ID=my-service
# Optional authentication
KAFKA_USERNAME=myuser
KAFKA_PASSWORD=mypassword
KAFKA_SASL_MECHANISM=sha256
# Optional TLS
KAFKA_CA_CERT=/path/to/ca-cert.pem
```

## 🚨 Error Handling

The library provides comprehensive error handling:

```go
reply, err := requestor.SendRequest(ctx, payload)
if err != nil {
    switch {
    case errors.Is(err, context.DeadlineExceeded):
        // Request timeout
    case errors.Is(err, reqreply.ErrNoPendingRequest):
        // Invalid correlation ID
    default:
        // Other errors
    }
    log.Printf("Request failed: %v", err)
}
```

## 🔄 Scaling

### Multiple Instances
Both requestor and responder support multiple instances via consumer groups:

```go
// Multiple requestor instances with same consumer group
requestor1 := reqreply.NewRequestor(client, registry, "req-topic", "reply-topic", "requestor-group")
requestor2 := reqreply.NewRequestor(client, registry2, "req-topic", "reply-topic", "requestor-group")

// Multiple responder instances with same consumer group
responder1 := reqreply.NewResponder(client, "req-topic", "reply-topic", "responder-group")
responder2 := reqreply.NewResponder(client, "req-topic", "reply-topic", "responder-group")
```

### Concurrent Requests
```go
var wg sync.WaitGroup
for i := 0; i < 10; i++ {
    wg.Add(1)
    go func(payload string) {
        defer wg.Done()
        reply, err := requestor.SendRequest(ctx, []byte(payload))
        // Handle response
    }(fmt.Sprintf("request-%d", i))
}
wg.Wait()
```

## 🛠️ Troubleshooting

### Common Issues

**Connection Timeout**
- Check `KAFKA_BROKERS` environment variable
- Verify network connectivity to Kafka brokers
- Ensure firewall rules allow Kafka ports

**Authentication Failures**
- Verify `KAFKA_USERNAME` and `KAFKA_PASSWORD`
- Check `KAFKA_SASL_MECHANISM` matches broker configuration
- Ensure user has proper permissions

**Topic Creation Errors**
- Verify admin permissions for the Kafka user
- Check if topics already exist (operation is idempotent)
- Ensure sufficient partitions and replication factors

**Missing Replies**
- Check responder is running and consuming from correct topic
- Verify correlation ID propagation
- Check consumer group lag

## 📄 License

This library is part of the Varnion Utils project.

## 🤝 Contributing

Contributions are welcome! Please ensure:
- Code follows existing patterns
- Error handling is comprehensive  
- Documentation is updated
- Examples are provided for new features

---

**Built with**: Franz-go v1.21.6 | **Go Version**: 1.25.0+ | **Pattern**: Request-Reply with Correlation IDs
