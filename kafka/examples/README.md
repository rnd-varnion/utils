# Kafka Request-Reply Examples

This directory contains complete, runnable examples demonstrating the **Request-Reply pattern** over Kafka using `github.com/rnd-varnion/utils/kafka`.

---

## 📐 Architecture Overview

The Request-Reply pattern allows microservices to communicate synchronously (request-response style) over asynchronous Kafka topics by attaching a unique `correlationId` header to each request.

```
+------------------+         Request Topic          +-------------------+
|                  | -----------------------------> |                   |
|    Requestor     |   Header: correlationId=123    |     Responder     |
|  (e.g., API Gateway)                              | (e.g., Order Svc) |
|                  | <----------------------------- |                   |
+------------------+          Reply Topic           +-------------------+
        |              Header: correlationId=123
        |
  [Correlation]
  [ Registry  ] ---> Matches reply correlationId back to waiting caller channel
```

1. **Requestor** generates a unique `correlationId` (16-byte random hex) and registers a channel in `CorrelationRegistry`.
2. **Requestor** sends the message to the **Request Topic** with the header `correlationId`.
3. **Responder** consumes the message from the **Request Topic**, invokes its handler function, and publishes the response to the **Reply Topic** with the same `correlationId` header.
4. **Requestor** consumes from the **Reply Topic**, extracts the `correlationId`, and delivers the payload to the waiting channel.

---

## 📂 Example Directories

| Example Directory | Description |
| :--- | :--- |
| [`reqreply/basic/`](./reqreply/basic/) | **All-in-One Demo**: Runs both Requestor and Responder in a single program. Demonstrates single requests, error handling, timeouts, and concurrent calls. |
| [`reqreply/responder/`](./reqreply/responder/) | **Responder Microservice**: A standalone service that listens to `service.orders.request` and publishes results to `service.orders.reply`. |
| [`reqreply/requestor/`](./reqreply/requestor/) | **Requestor Microservice**: A standalone client that sends requests to `service.orders.request` and awaits responses. |

---

## 🚀 How to Run the Examples

### Prerequisites

Ensure you have a Kafka broker running locally at `localhost:9092` (or set `KAFKA_BROKERS` env variable).

### 1. Run the All-in-One Example

This is the easiest way to see everything in action:

```bash
cd kafka
go run ./examples/reqreply/basic
```

**Expected Output:**
```
==================================================
 Kafka Request-Reply Pattern Example (All-in-One)
==================================================

--- DEMO 1: Single Successful Request ---
[REQUESTOR] ✅ Reply Received: UserID=usr-101, Name=John Doe (usr-101), Status=ACTIVE

--- DEMO 2: Error Response (User Not Found) ---
[REQUESTOR] ⚠️ Error Reply Received: User Not Found

--- DEMO 3: Request Timeout ---
[REQUESTOR] ❌ Request failed after 1s: context deadline exceeded

--- DEMO 4: Concurrent Requests ---
[REQUESTOR] ✅ Reply Received: UserID=usr-201...
[REQUESTOR] ✅ Reply Received: UserID=usr-202...
...
```

---

### 2. Run as Independent Microservices

In production, Requestor and Responder run as separate microservices across different servers.

**Terminal 1 — Start the Responder Service:**
```bash
go run ./kafka/examples/reqreply/responder
```

**Terminal 2 — Run the Requestor Client:**
```bash
go run ./kafka/examples/reqreply/requestor
```

---

## 💡 Key Patterns & Best Practices

### 1. Defining Request/Response Schemas
Always use structured data (e.g. JSON structs) for predictable payload serialization:

```go
type OrderRequest struct {
    Action  string `json:"action"`
    OrderID string `json:"order_id"`
}

type OrderResponse struct {
    Success bool   `json:"success"`
    Data    string `json:"data,omitempty"`
    Error   string `json:"error,omitempty"`
}
```

### 2. Setting Context Timeouts
Always set explicit timeouts using Go contexts when calling `SendRequest`:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

reply, err := requestor.SendRequest(ctx, payload)
if err != nil {
    // Handle timeout or transmission failure
}
```

### 3. Graceful Shutdown
Clean up registry memory tickers and Kafka consumers on exit:

```go
defer client.Close()
defer registry.Close()
defer requestor.Stop()
defer responder.Stop()
```
