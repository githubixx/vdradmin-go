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

- **Go 1.23+**: Latest features, enhanced routing
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

### Unit Tests

- Test domain logic in isolation
- Mock ports (interfaces)
- Fast, no external dependencies

### Integration Tests

- Test adapters with real dependencies
- Use test containers for VDR
- Slower but verify actual behavior

### End-to-End Tests

- Test full HTTP flow
- Use test VDR instance
- Verify user-facing functionality

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
