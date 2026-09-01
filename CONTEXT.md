# Kafka Request-Reply Library - Implementation Context

## Project Overview
Converting the current Kafka request-reply demo application into a reusable library for a utils project.

## Current Codebase Analysis

### Structure
```
request-reply/
├── main.go           # Demo orchestration
├── connection.go     # Kafka client configuration  
├── correlation.go    # Correlation ID registry
├── requestor.go      # Service A implementation
├── responder.go      # Service B implementation
├── topics.go         # Topic management utilities
└── .env             # Configuration
```

### Technology Stack
- Go 1.25.0
- `github.com/twmb/franz-go` (v1.21.5) - Kafka client
- `github.com/twmb/franz-go/pkg/kadm` (v1.18.0) - Kafka admin API
- `github.com/joho/godotenv` (v1.5.1) - Environment variables

### Current Implementation
- **Requestor Service**: Sends requests with correlation IDs, waits for replies
- **Responder Service**: Processes requests, sends replies with matching correlation IDs
- **Registry System**: Thread-safe correlation ID matching
- **Connection Management**: SASL/TLS authentication support
- **Topic Management**: Auto-creation of request/reply topics

## Library Design Requirements

### Target Structure: Option B (Multi-Package Modular)
```
your-utils/
├── go.mod
├── kafka/
│   ├── reqreply/
│   │   ├── client.go       # Connection management
│   │   ├── requestor.go    # Request sending logic
│   │   ├── responder.go    # Response handling logic
│   │   ├── correlation.go  # Correlation ID registry
│   │   ├── topics.go       # Topic management
│   │   └── config.go       # Configuration handling
│   └── common/
│       └── config.go        # Shared Kafka utilities
└── examples/
    ├── simple-reqreply.go
    └── advanced-usage.go
```

### Key Constraint: Environment-Based Configuration
- **Important**: The existing utils project reads configuration directly from `.env`
- Library must integrate with existing `.env` loading patterns
- Configuration should be automatic, not require manual config passing

### Architecture Goals
1. **Clean API Surface**: Simple interfaces for requesting and responding
2. **Modular Design**: Separate packages for concerns (client, requestor, responder)
3. **Environment Integration**: Seamless `.env` configuration loading
4. **Production Ready**: Proper error handling, timeouts, cleanup
5. **Well Documented**: Clear examples and usage patterns

## Key Components to Extract

### 1. Connection Layer (`connection.go`)
- Kafka client factory with authentication support
- SASL SCRAM-SHA-256/512 + TLS support
- Environment-based configuration loading
- Connection health checking

**Current Environment Variables:**
- `KAFKA_BROKERS` - Comma-separated broker list
- `KAFKA_USERNAME` - SASL username (optional)
- `KAFKA_PASSWORD` - SASL password (optional)
- `KAFKA_CA_CERT` - Path to CA certificate (optional)
- `KAFKA_SASL_MECHANISM` - "sha256" or "sha512" (optional)

### 2. Correlation Registry (`correlation.go`)
- Thread-safe correlation ID mapping
- Channel-based reply delivery
- Automatic cleanup of stale requests
- Random 16-byte correlation ID generation

### 3. Requestor Service (`requestor.go`)
- Sends requests with correlation ID headers
- Blocking request-reply pattern with configurable timeout
- Reply consumer and dispatcher
- Consumer group management

**Current Hardcoded Values (should be configurable):**
- Request topic: `"data-request"`
- Reply topic: `"data-reply"`
- Consumer group: `"request-reply-requestor"`
- Timeout: `10 * time.Second`

### 4. Responder Service (`responder.go`)
- Consumes requests from request topic
- Processes requests and sends replies
- Correlation ID propagation
- Consumer group management

**Current Hardcoded Values (should be configurable):**
- Request topic: `"data-request"`
- Reply topic: `"data-reply"`  
- Consumer group: `"request-reply-responder"`
- Business logic: `"pong: " + strings.ToUpper(payload)`

### 5. Topic Management (`topics.go`)
- Idempotent topic creation
- Uses Kafka admin API
- Auto-creates missing topics with configurable partition/replication

## Implementation Strategy

### Phase 1: Package Structure
1. Create `kafka/reqreply` package structure
2. Move core functionality from `main` package to library package
3. Export public APIs, keep internals private

### Phase 2: Configuration Design
1. Design configuration struct that works with `.env` loading
2. Support environment variable overrides
3. Provide sensible defaults
4. Match existing project configuration patterns

### Phase 3: API Design
1. **Requestor API**: Simple `SendRequest()` interface
2. **Responder API**: Handler-based pattern for business logic
3. **Client API**: Connection management and lifecycle
4. **Topic API**: Administrative utilities

### Phase 4: Business Logic Abstraction
1. Extract hardcoded business logic from responder
2. Allow users to inject their own request handlers
3. Support different serialization formats

### Phase 5: Examples and Documentation
1. Create simple usage examples
2. Document all public APIs
3. Provide integration patterns

## Design Considerations

### Configuration Approach
Since the project reads from `.env`:
- Library should auto-discover configuration from environment
- Allow programmatic overrides where needed
- Support multiple configuration sources (env vars, config files, programmatic)

### API Style
Should the library expose:
1. **High-level API**: Simple `SendRequest()` / `HandleRequest()` methods
2. **Low-level API**: Fine-grained control over client, consumer, producer
3. **Hybrid**: Both approaches available

### Error Handling
- Proper error propagation
- Context-based cancellation
- Timeout handling
- Connection failure recovery

### Lifecycle Management
- Graceful shutdown
- Resource cleanup
- Connection pooling
- Consumer group management

## Next Steps

1. **Confirm package structure** with existing project patterns
2. **Define configuration interface** that matches `.env` loading
3. **Design public API surface** for library users
4. **Extract business logic** from responder into handler interface
5. **Create examples** demonstrating typical usage patterns

## Open Questions

1. What is the exact structure of the existing utils project?
2. How do existing packages currently load `.env` configuration?
3. Should the library support multiple serialization formats (JSON, protobuf, etc.)?
4. What level of backward compatibility is needed with the current demo?
5. Should the library include metrics/observability features?

---

**Generated**: 2026-09-01  
**Purpose**: Implementation context for converting Kafka request-reply demo to reusable library  
**Key Constraint**: Must integrate with existing `.env` configuration patterns in utils project