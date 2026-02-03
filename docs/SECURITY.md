# Security

## Path Traversal Protection

vdradmin-go includes protection against path traversal attacks (CWE-22) to prevent unauthorized access to files outside configured directories.

### Protected Areas

#### 1. Recording Paths

Recording paths are validated to ensure they remain within the configured `vdr.video_dir`:

- Absolute paths are rejected
- Path traversal sequences (`..`) are blocked
- Backslashes are rejected on Unix systems
- All paths are cleaned and validated before use

**Implementation**: See `validateRecordingPath()` and `isPathWithinBase()` in `internal/adapters/primary/http/handlers.go`

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

**Implementation**: See `NormalizePreview()` in `internal/application/archive/archive.go`

### Admin-Only Configuration

The following configuration paths are considered safe because they can only be modified by administrators:

- `vdr.config_dir` - VDR configuration directory
- `server.tls.cert_file` - TLS certificate file (can be anywhere)
- `server.tls.key_file` - TLS private key file (can be anywhere)
- `archive.base_dir` - Archive output base directory
- `archive.profiles[].base_dir` - Profile-specific archive directories

These paths are not subject to runtime validation since they require admin privileges to modify and are part of the application's trusted configuration.

### Testing

Comprehensive tests ensure path validation works correctly:

```bash
go test ./internal/adapters/primary/http -run TestIsPathWithinBase
go test ./internal/adapters/primary/http -run TestValidateRecordingPath
```

### Security Best Practices

1. **Never disable path validation** - The validation logic should not be bypassed
2. **Always use absolute paths** for base directories in configuration
3. **Regular updates** - Keep dependencies updated for security patches
4. **Access control** - Ensure proper authentication/authorization for admin functions
5. **Least privilege** - Run the application with minimal required permissions

### Reporting Security Issues

If you discover a security vulnerability, please report it privately to the maintainers rather than opening a public issue.
