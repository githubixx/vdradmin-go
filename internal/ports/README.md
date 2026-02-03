# Port Layer Contract Tests

This package provides a reusable contract test suite for VDRClient implementations.

## Purpose

Contract tests ensure that all implementations of the `VDRClient` interface behave consistently:
- Real SVDRP client implementations
- Mock implementations used in tests
- Fake implementations for integration testing
- Alternative backends (if added in the future)

## Benefits

1. **Consistency**: All implementations follow the same behavioral contracts
2. **Safety**: Mock implementations are validated to match real behavior
3. **Documentation**: Tests serve as executable specification of expected behavior
4. **Refactoring**: Changes to implementations are validated against contracts
5. **New Implementations**: Provides ready-made test suite for new backends

## Usage

### Testing Your VDRClient Implementation

```go
func TestMyClient_ContractCompliance(t *testing.T) {
    factory := func() (ports.VDRClient, func()) {
        client := NewMyClient("localhost", 6419, 5*time.Second)
        cleanup := func() {
            client.Close()
        }
        return client, cleanup
    }

    ports.RunVDRClientContractTests(t, factory)
}
```

### Testing SVDRP Client (Integration Test)

To run contract tests against the real SVDRP client, you need a running VDR instance or test container:

```go
func TestSVDRPClient_ContractCompliance(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }

    // Start testcontainer with SVDRP stub
    container := startSVDRPTestContainer(t)
    defer container.Terminate(context.Background())

    factory := func() (ports.VDRClient, func()) {
        host, port := container.Host(), container.Port()
        client := svdrp.NewClient(host, port, 5*time.Second)
        cleanup := func() {
            client.Close()
        }
        return client, cleanup
    }

    ports.RunVDRClientContractTests(t, factory)
}
```

## Contract Test Categories

The contract suite validates:

### 1. Connection Management
- Connect/Ping/Close lifecycle
- Connection error handling
- Multiple Close calls (idempotency)

### 2. Channel Operations
- GetChannels returns valid channel list
- Channel structure validation (ID, Number, Name)

### 3. EPG Operations
- GetEPG returns valid events
- Event time consistency (Start < Stop)
- Empty channel ID handling

### 4. Timer CRUD
- GetTimers returns valid timer list
- CreateTimer/UpdateTimer/DeleteTimer behavior
- Nil/invalid input handling

### 5. Recording Operations
- GetRecordings returns valid recording list
- GetRecordingDir path resolution
- DeleteRecording behavior
- Empty path handling

### 6. Current Channel
- GetCurrentChannel/SetCurrentChannel
- Empty channel ID handling

### 7. Remote Control
- SendKey with valid/invalid keys
- Empty key handling

### 8. Context Handling
- Context cancellation propagation
- Context timeout enforcement

### 9. Error Handling
- Domain error types returned
- Nil context behavior

## Implementation Guidelines

When implementing `VDRClient`:

1. **Always return non-nil slices** from Get* methods (even if empty)
2. **Handle nil pointers gracefully** in Create/Update methods
3. **Validate input** and return appropriate domain errors
4. **Respect context cancellation** in long-running operations
5. **Be idempotent** where possible (e.g., multiple Close calls)
6. **Use domain errors** (`domain.ErrNotFound`, `domain.ErrInvalidInput`, etc.)

## Using the Canonical Mock

The `MockVDRClient` provides two usage patterns:

### Builder Pattern (Recommended for simple cases)

```go
mock := ports.NewMockVDRClient().
    WithChannels([]domain.Channel{{ID: "1", Number: 1, Name: "Test"}}).
    WithTimers([]domain.Timer{{ID: 1, Title: "Timer"}}).
    WithCurrentChannel("1")

// Use in tests
service := NewMyService(mock)
```

### Function Fields (For complex behavior)

```go
mock := &ports.MockVDRClient{
    GetChannelsFunc: func(ctx context.Context) ([]domain.Channel, error) {
        // Custom logic
        return channels, nil
    },
    CreateTimerFunc: func(ctx context.Context, timer *domain.Timer) error {
        if timer == nil {
            return domain.ErrInvalidInput // Validate input
        }
        // Custom create logic
        return nil
    },
}
```

See [mock_vdr_client.go](./mock_vdr_client.go) for the full implementation.

## Running Contract Tests

```bash
# Run all port tests
go test ./internal/ports/...

# Run with verbose output
go test ./internal/ports/... -v

# Run only contract tests
go test ./internal/ports/... -run Contract
```

## Adding New Contract Tests

When extending the VDRClient interface:

1. Add new test function to `vdr_contract_test.go`:
```go
func testNewFeature(t *testing.T, factory ClientFactory) {
    t.Run("FeatureName_Scenario", func(t *testing.T) {
        client, cleanup := factory()
        defer cleanup()
        // ... test logic
    })
}
```

2. Register it in `RunVDRClientContractTests`:
```go
func RunVDRClientContractTests(t *testing.T, factory ClientFactory) {
    // ... existing tests
    t.Run("NewFeature", func(t *testing.T) { testNewFeature(t, factory) })
}
```

3. All implementations automatically inherit the new test

## Related

- [VDR Interface](./vdr.go) - The VDRClient interface definition
- [SVDRP Client](../adapters/secondary/svdrp/client.go) - Real SVDRP implementation
- [Service Mocks](../application/services/mock_vdr_client_test.go) - Service-layer mocks
