# Security

## Dependency Security

Docker SDK dependencies are used only by the Docker-backed integration tests,
not by the application binaries. Testcontainers uses the supported
`github.com/moby/moby/api` and `github.com/moby/moby/client` modules instead of
the deprecated `github.com/docker/docker` module. The legacy module has
advisories without a fixed version, including
[GO-2026-4883](https://pkg.go.dev/vuln/GO-2026-4883).

The archive, compression, and SSH dependencies are also pinned to patched
versions. The SSH fixes require Go 1.26 or newer.

Module-level scanners may still report
[GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932) for the unmaintained
`golang.org/x/crypto/openpgp` package. The Docker test stack uses SSH from that
module, but does not import OpenPGP; this module-only advisory is distinct from
a vulnerability in an imported package or linked symbol.

Include integration tests when checking dependency security:

```bash
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
go run golang.org/x/vuln/cmd/govulncheck@latest -tags=integration -test ./internal/integration
```

Verify that the scanner includes transitive modules, not only this repository.
Scanning compiled binaries also checks the actual linked dependency versions:

```bash
scan_dir=$(mktemp -d)
go build -o "$scan_dir/vdradmin-go" ./cmd/vdradmin-go
go build -o "$scan_dir/vdradmin-go-mcp" ./cmd/vdradmin-go-mcp
go test -tags=integration -c -o "$scan_dir/integration.test" ./internal/integration
go run golang.org/x/vuln/cmd/govulncheck@latest -mode=binary "$scan_dir/vdradmin-go"
go run golang.org/x/vuln/cmd/govulncheck@latest -mode=binary "$scan_dir/vdradmin-go-mcp"
go run golang.org/x/vuln/cmd/govulncheck@latest -mode=binary "$scan_dir/integration.test"
rm "$scan_dir/vdradmin-go" "$scan_dir/vdradmin-go-mcp" "$scan_dir/integration.test"
rmdir "$scan_dir"
```

Updating the Go SDK does not update Docker Engine or its container runtime.
Keep the Docker installation on development and CI hosts patched separately.

## Path Traversal Protection

vdradmin-go includes protection against path traversal attacks (CWE-22) to prevent unauthorized access to files outside configured directories.

### Protected Areas

#### 1. Recording Paths

Recording paths are validated at multiple levels to ensure they remain within the configured `vdr.video_dir`:

**User-provided recording IDs** (relative paths): Validated by `validateRecordingPath()`

- Absolute paths are rejected
- Path traversal sequences (`..`) are blocked
- Backslashes are rejected on Unix systems
- All paths are cleaned and validated before use

**VDR-returned recording directories** (absolute paths): Validated by `validateRecordingDir()`

- Ensures returned paths are within the configured video directory
- Protects against compromised or malicious VDR instances
- Validates before reading info files or accessing recording data

**Implementation**: See `validateRecordingPath()`, `validateRecordingDir()`, and `isPathWithinBase()` in `internal/adapters/primary/http/handlers.go`

#### 2. HLS Streaming

HLS proxy channel numbers and segment names are validated:

- Directory separators (`/`, `\`) are blocked
- Path traversal sequences (`..`) are blocked
- Only simple alphanumeric identifiers are allowed

**Implementation**: See `GetPlaylist()`, `GetSegment()`, and `ensureStream()` in `internal/adapters/primary/http/hls_proxy.go`

#### 3. Archive Operations

Archive paths are validated during preview and execution:

- Target directories must be within configured archive base directories
- Video and info file paths must be within the target directory
- Path cleaning is applied to all user-provided paths
- Defensive validation checks for path traversal sequences in all operations

**Implementation**: See `NormalizePreview()` and `validatePath()` in `internal/application/archive/archive.go`; validation applied in `runArchive()`, `DiscoverSegments()`, and `WriteConcatList()`

### Admin-Only Configuration

The following configuration paths are considered safe because they can only be modified by administrators:

- `vdr.config_dir` - VDR configuration directory
- `server.tls.cert_file` - TLS certificate file (can be anywhere)
- `server.tls.key_file` - TLS private key file (can be anywhere)
- `archive.base_dir` - Archive output base directory
- `archive.profiles[].base_dir` - Profile-specific archive directories

These paths are not subject to runtime validation since they require admin privileges to modify and are part of the application's trusted configuration.

### MCP HTTP Listener

`vdradmin-go-mcp --transport=http` is intentionally unauthenticated in the initial release. Its default listener is `127.0.0.1:8081` and must remain loopback-only unless a trusted reverse proxy or other access-control boundary protects it. Do not expose the `/mcp` endpoint directly to a LAN or the public internet.

The MCP service exposes only read-only EPG search today. Future authentication will be implemented as HTTP middleware so it applies consistently before MCP tool execution.

### Testing

Comprehensive tests ensure path validation works correctly:

```bash
# Test path validation utilities
go test ./internal/adapters/primary/http -run TestIsPathWithinBase
go test ./internal/adapters/primary/http -run TestValidateRecordingPath
go test ./internal/adapters/primary/http -run TestValidateRecordingDir

# Test archive path validation
go test ./internal/application/archive -run TestValidatePath
```

### Security Best Practices

1. **Never disable path validation** - The validation logic should not be bypassed
2. **Always use absolute paths** for base directories in configuration
3. **Regular updates** - Keep dependencies updated for security patches
4. **Access control** - Ensure proper authentication/authorization for admin functions
5. **Least privilege** - Run the application with minimal required permissions

### Reporting Security Issues

If you discover a security vulnerability, please report it privately to the maintainers rather than opening a public issue.
