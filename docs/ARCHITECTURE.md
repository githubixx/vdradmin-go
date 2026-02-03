# vdradmin-go Architecture

## Overview

vdradmin-go follows **Hexagonal Architecture** (Ports and Adapters pattern) for maximum maintainability, testability, and flexibility.

## Architecture Layers

```plain
┌─────────────────────────────────────────────────────────┐
│                      Primary Adapters                   │
│                   (Incoming Requests)                   │
│                                                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐   │
│  │   HTTP/Web   │  │     CLI      │  │     API      │   │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘   │
└─────────┼──────────────────┼──────────────────┼─────────┘
          │                  │                  │
          └──────────────────┴──────────────────┘
                             │
          ┌──────────────────▼──────────────────┐
          │         Primary Ports               │
          │         (Interfaces)                │
          └──────────────────┬──────────────────┘
                             │
          ┌──────────────────▼──────────────────┐
          │        Application Layer            │
          │                                     │
          │  ┌─────────────────────────────┐    │
          │  │   Services (Use Cases)      │    │
          │  │  - EPGService               │    │
          │  │  - TimerService             │    │
          │  │  - RecordingService         │    │
          │  │  - AutoTimerService         │    │
          │  └─────────────────────────────┘    │
          └──────────────────┬──────────────────┘
                             │
          ┌──────────────────▼──────────────────┐
          │         Domain Layer                │
          │                                     │
          │  ┌─────────────────────────────┐    │
          │  │   Domain Models             │    │
          │  │  - Channel                  │    │
          │  │  - EPGEvent                 │    │
          │  │  - Timer                    │    │
          │  │  - Recording                │    │
          │  │  - AutoTimer                │    │
          │  └─────────────────────────────┘    │
          │  ┌─────────────────────────────┐    │
          │  │   Domain Logic              │    │
          │  │  - Business Rules           │    │
          │  │  - Validation               │    │
          │  └─────────────────────────────┘    │
          └──────────────────┬──────────────────┘
                             │
          ┌──────────────────▼──────────────────┐
          │        Secondary Ports              │
          │         (Interfaces)                │
          │  - VDRClient                        │
          │  - CacheRepository                  │
          └──────────────────┬──────────────────┘
                             │
┌─────────────────────────────▼───────────────────────────┐
│                   Secondary Adapters                    │
│                  (Outgoing Requests)                    │
│                                                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐   │
│  │ SVDRP Client │  │   File I/O   │  │   Database   │   │
│  └──────────────┘  └──────────────┘  └──────────────┘   │
└─────────────────────────────────────────────────────────┘
```

## Directory Structure

```plain
vdradmin-go/
├── cmd/
│   └── vdradmin/              # Application entry point
│       └── main.go
├── internal/
│   ├── domain/                # Domain layer (core business logic)
│   │   ├── models.go          # Domain entities
│   │   └── errors.go          # Domain errors
│   ├── ports/                 # Interfaces (hexagonal ports)
│   │   └── vdr.go             # VDR client interface
│   ├── application/           # Application layer (use cases)
│   │   └── services/
│   │       ├── epg_service.go
│   │       ├── timer_service.go
│   │       ├── recording_service.go
│   │       └── autotimer_service.go
│   ├── adapters/              # Implementations (hexagonal adapters)
│   │   ├── primary/           # Incoming adapters
│   │   │   └── http/
│   │   │       ├── handlers.go
│   │   │       ├── middleware.go
│   │   │       └── server.go
│   │   └── secondary/         # Outgoing adapters
│   │       └── svdrp/
│   │           └── client.go  # SVDRP protocol implementation
│   └── infrastructure/        # Cross-cutting concerns
│       └── config/
│           └── config.go
├── web/
│   ├── templates/             # HTML templates
│   └── static/                # CSS, minimal JS
├── configs/                   # Configuration files
├── deployments/               # Docker, systemd
└── docs/                     # Documentation
```

## Key Design Principles

### 1. Dependency Inversion

Dependencies flow inward toward the domain:

- **Adapters** depend on **Ports** (interfaces)
- **Domain** has no dependencies
- **Application Services** depend on **Domain** and **Ports**

### 2. Separation of Concerns

Each layer has a single responsibility:

- **Domain**: Business entities and rules
- **Application**: Use cases and orchestration
- **Adapters**: External communication
- **Infrastructure**: Technical concerns

### 3. Testability

All dependencies are interfaces, enabling:

- Easy mocking in unit tests
- Test doubles for integration tests
- Isolated testing of each layer

### 4. Flexibility

Adapters are interchangeable:

- Replace HTTP with gRPC
- Replace SVDRP with REST API
- Replace in-memory cache with Redis

## Data Flow

### Example: Creating a Timer from EPG

1. **HTTP Handler** receives POST request
2. **Handler** calls **TimerService.CreateTimerFromEPG()**
3. **TimerService** uses **EPGService** to find event
4. **EPGService** calls **VDRClient** port
5. **SVDRP Adapter** implements **VDRClient** interface
6. **SVDRP Client** communicates with VDR
7. Response flows back through layers

```go
HTTP → Handler → TimerService → EPGService → VDRClient (interface) → SVDRP → VDR
                      ↓
                  Domain Models
```

## Technology Choices

### Core

- **Go 1.25+**: Latest features, enhanced routing
- **net/http**: Standard library routing (Go 1.22+ enhancements)
- **html/template**: Standard library templating
- **slog**: Standard library structured logging

### Frontend

- **htmx**: Dynamic UI without JavaScript frameworks
- **Modern CSS**: Grid, Flexbox, CSS Variables
- **Minimal JavaScript**: Only from htmx

### Configuration

- **YAML**: Human-readable configuration
- **Validation**: Built-in config validation

### Deployment

- **Docker**: Containerization
- **systemd**: Native Linux service

## Testing Strategy

vdradmin-go employs a multi-layered testing approach with 250+ tests across 7 packages, ensuring reliability from domain logic to external integrations.

### Unit Tests

**Domain Layer** (`internal/domain/*_test.go`):

- 86 test cases validating business rules and model behavior
- Property-based tests using `testing/quick` for invariant checking
- Tests for channel IDs, timer validation, recording paths, weekday masks
- Zero external dependencies, extremely fast

**Application Services** (`internal/application/services/*_test.go`):

- Service orchestration with canonical mock implementations
- Cache behavior validation (EPG, recordings, timers)
- Business logic edge cases (timer conflicts, recording grouping)
- Uses interface mocks, no real VDR required

**HTTP Handlers** (`internal/adapters/primary/http/*_test.go`):

- Request/response validation
- Form parsing and validation
- Error handling scenarios
- Uses httptest for isolated handler testing

### Port Contract Tests

**Reusable Test Suite** (`internal/ports/vdr_contract_test.go`):

- 40 test cases defining expected VDRClient behavior
- Validates all implementations (mock, SVDRP, future adapters)
- Ensures consistent error handling and data structures
- Prevents implementation drift

Example usage:

```go
func TestMyClient_ContractCompliance(t *testing.T) {
    factory := func(t *testing.T) ports.VDRClient {
        return NewMyClient("host", 6419)
    }
    ports.RunVDRClientContractTests(t, factory)
}
```

### Integration Tests

**SVDRP Protocol Tests** (`internal/integration/svdrp_integration_test.go`):

- Uses testcontainers-go to spin up real SVDRP stub server
- 14 tests validating full protocol communication
- Tests channel retrieval, timer CRUD, connection resilience
- Concurrent operation safety
- Context timeout handling
- Requires Docker, slower but validates real protocol behavior

Run with: `go test -tags=integration ./internal/integration -v`

### Property-Based Testing

**Invariant Validation** (`internal/domain/models_property_test.go`):

- 10 property-based tests using `testing/quick`
- Validates domain invariants across random inputs
- Tests that succeed for all valid inputs in the domain
- Examples: channel number ranges, timer priority bounds, recording size validation
- Excellent for finding edge cases

### Fuzz Testing

**Protocol Parsing** (`internal/adapters/secondary/svdrp/fuzz_test.go`):

- 4 fuzz functions for SVDRP response parsing
- Tests: ParseSVDRPResponse, ParseChannelID, ParseEPGDescription, ParseTimestamp
- Uses Go 1.18+ native fuzzing
- Run with: `go test -fuzz=FuzzParseSVDRPResponse -fuzztime=5m ./internal/adapters/secondary/svdrp/`
- Discovers malformed input handling and crash scenarios

### Race Detection

**Concurrency Safety** (`internal/application/services/race_test.go`):

- 9 concurrent access tests for service layer caches
- Validates thread-safe operations under load
- Tests EPG cache, recording cache, timer operations
- Automatically enabled by `make test` via `-race` flag
- Has discovered real production bugs (data races in cache access)

Example discovered bug: RecordingService reading `cacheExpiry` without lock protection

### Test Execution

```bash
# All unit tests with race detection (default)
make test

# Coverage report (generates coverage.html)
make test-coverage

# Integration tests only (requires Docker)
go test -tags=integration ./internal/integration -v

# Race tests only
go test -race ./internal/application/services/... -run Race -v

# Fuzz testing (optional, time-intensive)
go test -fuzz=. -fuzztime=10m ./internal/adapters/secondary/svdrp/

# Specific test pattern
go test ./internal/domain -run TestTimer -v
```

### Test Coverage Philosophy

**What We Test:**

- Domain logic and business rules (high coverage)
- Port interface contracts (ensures consistency)
- Protocol parsing (fuzz testing for robustness)
- Concurrent access patterns (race detection)
- Critical user flows (integration tests)

**What We Don't Over-Test:**

- Simple getters/setters
- Framework code (Go stdlib, third-party libraries)
- Unreachable error paths

**Coverage Metrics:**

- Domain: High coverage via unit + property tests
- Services: ~47% (focus on cache logic and orchestration)
- HTTP Handlers: ~36% (focus on critical paths)
- SVDRP Adapter: ~57% (protocol parsing well-covered)
- Ports: ~79% (contract tests ensure compliance)

Low percentages often reflect error handling paths requiring specific VDR server states to trigger.

## Security

### Authentication

- HTTP Basic Auth
- Configurable admin/guest users
- Subnet-based auth bypass

### Authorization

- Role-based access control
- Admin/guest roles
- Middleware enforcement

### Security Headers

- X-Content-Type-Options
- X-Frame-Options
- X-XSS-Protection
- Referrer-Policy

### Input Validation

- Form validation
- Domain-level validation
- SQL injection prevention (no SQL used)
- XSS prevention (template auto-escaping)

## Performance

### Caching

- EPG cache (configurable expiry)
- Recording cache (configurable expiry)
- In-memory caching with TTL

### Concurrency

- Goroutines for concurrent requests
- Connection pooling for SVDRP
- Non-blocking I/O

### Compression

- gzip middleware for HTTP responses
- Reduced bandwidth usage

## Observability

### Logging

- Structured logging with slog
- Request/response logging
- Error logging with context

### Metrics (Future)

- Prometheus metrics
- Request duration
- Error rates
- Cache hit rates

### Tracing (Future)

- OpenTelemetry integration
- Distributed tracing

## Extensibility

### Adding New Features

1. **Define Domain Model** in `internal/domain/`
2. **Create Port** in `internal/ports/` if needed
3. **Implement Service** in `internal/application/services/`
4. **Create HTTP Handler** in `internal/adapters/primary/http/`
5. **Add Routes** in `server.go`
6. **Create Templates** in `web/templates/`

### Example: Adding Channel Management

```go
// 1. Domain
type ChannelGroup struct {
    ID       int
    Name     string
    Channels []string
}

// 2. Port (if needed)
type ChannelManager interface {
    GetGroups(ctx context.Context) ([]ChannelGroup, error)
}

// 3. Service
type ChannelService struct {
    vdrClient ports.VDRClient
}

// 4. Handler
func (h *Handler) ChannelGroups(w http.ResponseWriter, r *http.Request) {
    // Implementation
}

// 5. Route
mux.Handle("GET /channels", chain(handler.ChannelGroups, commonMiddleware...))
```

## Best Practices

1. **Keep domain pure**: No external dependencies in domain layer
2. **Use interfaces**: Define ports as interfaces
3. **Validate early**: Validate at service layer
4. **Handle errors**: Wrap errors with context
5. **Log appropriately**: Info for operations, Error for failures
6. **Test thoroughly**: Unit tests for logic, integration for adapters
7. **Document decisions**: Update architecture docs for major changes

## Future Enhancements

- [ ] WebSocket support for real-time updates
- [ ] GraphQL API as alternative to REST
- [ ] Prometheus metrics export
- [ ] OpenTelemetry tracing
- [ ] Multi-VDR support
- [ ] Recording streaming
- [ ] Mobile-responsive UI improvements
- [ ] PWA (Progressive Web App) support
