# Security

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
