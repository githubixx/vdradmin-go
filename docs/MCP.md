# MCP Server

`vdradmin-go-mcp` exposes read-only VDR electronic program guide (EPG) and recording search through the [Model Context Protocol](https://modelcontextprotocol.io/). It uses the same VDR connection and caches as the web application. EPG search also honors `vdr.wanted_channels` filtering.

The server provides two tools: `search_epg` for VDR EPG data and `search_recordings` for completed recordings. Neither tool uses an external show catalogue or requires `vdr-plugin-epgsearch`.

## Build

Build both binaries:

```bash
make build
```

The MCP executable is `build/vdradmin-go-mcp`. It accepts the shared `config.yaml` file:

```bash
build/vdradmin-go-mcp --config /var/lib/vdradmin-go/config.yaml
```

Direct invocation defaults to the local stdio transport. Do not run it manually in an interactive terminal: it waits for JSON-RPC messages on standard input. Operational logs use standard error so they do not corrupt the MCP protocol on standard output.

Use `build/vdradmin-go-mcp --version` to print build metadata.

## EPG Search Tool

`search_epg` accepts:

- `pattern` (required): phrase or regular expression.
- `mode`: `phrase` (default) or `regex`.
- `matchCase`: case-sensitive matching when `true`.
- `inTitle`, `inSubtitle`, `inDescription`: fields to search. When all are omitted or false, all fields are searched.
- `channelIds`: optional list of VDR channel IDs.
- `startsAt`, `endsAt`: optional RFC3339 time bounds. Events overlapping the requested window are included.
- `resultLimit`: optional result count from `1` to `200`; the default is `50`.

Results are ordered by event start time and channel. Each result contains the VDR event and channel IDs, channel metadata, title/subtitle/description, RFC3339 start and end times, and duration. The structured response also includes `total` and `truncated` so clients can recognize a bounded result set.

## Recording Search Tool

`search_recordings` accepts:

- `pattern` (required): phrase to search for. It is trimmed and must contain at least 3 characters.
- `inSubtitle`: search subtitles in addition to titles.
- `inPath`: search recording paths in addition to titles.
- `inDescription`: search descriptions in addition to titles.
- `inChannel`: search channel names in addition to titles.
- `sortBy`: optional sort order. Supported values are `date` (default, newest first), `name`, `date_oldest`, and `length`.
- `resultLimit`: optional result count from `1` to `200`; the default is `50`.

Recording searches are phrase-only and case-insensitive. Titles are always searched; optional fields are searched only when their flags are enabled. Empty or too-short patterns return a tool error instead of an unfiltered recording list.

Results contain the recording path, title, optional subtitle/description/channel, RFC3339 recording date, length in seconds, size in bytes, and folder flag. The structured response also includes `total` and `truncated` so clients can recognize a bounded result set. Local filesystem paths are not exposed.

## Streamable HTTP

To run a persistent local server, select HTTP mode:

```bash
build/vdradmin-go-mcp --transport=http --config /var/lib/vdradmin-go/config.yaml
```

The endpoint is `http://127.0.0.1:8081/mcp` by default. Configure another listener only when a trusted network boundary is already in place:

```yaml
mcp:
  host: "127.0.0.1"
  port: 8081
```

HTTP mode uses stateless Streamable HTTP and has no authentication in this release. Do not bind it to a LAN or public address without a reverse proxy or another access-control layer. Authentication belongs in HTTP middleware so future OAuth support will not alter the EPG search service or tool contract.

## systemd

The included [vdradmin-go-mcp.service](../deployments/systemd/vdradmin-go-mcp.service) starts the HTTP transport for URL-based MCP clients:

```bash
sudo install -m 0644 deployments/systemd/vdradmin-go-mcp.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now vdradmin-go-mcp
sudo systemctl status vdradmin-go-mcp
```

The packaged unit starts `/usr/bin/vdradmin-go-mcp` with configuration in `/var/lib/vdradmin-go/config.yaml`. The service shares the existing `vdr` user, working directory, VDR ordering, and filesystem hardening conventions.

## Client Setup

Use an absolute path to both the executable and configuration file for local stdio setups.

### VS Code

Create `.vscode/mcp.json` for a workspace-specific server, or run `MCP: Open User Configuration` for a user-level server:

```json
{
  "servers": {
    "vdradmin": {
      "type": "stdio",
      "command": "/usr/bin/vdradmin-go-mcp",
      "args": ["--config", "/var/lib/vdradmin-go/config.yaml"]
    }
  }
}
```

For the systemd-managed HTTP service, use:

```json
{
  "servers": {
    "vdradmin": {
      "type": "http",
      "url": "http://127.0.0.1:8081/mcp"
    }
  }
}
```

VS Code asks you to trust newly configured local servers. Use `MCP: List Servers` to inspect status and logs.

In the VS Code chat window you can then ask `How to use the vdradmin-go-mcp server?` e.g. Then you get something like this:

```plain
vdradmin-go-mcp provides read-only VDR search tools: search_epg and search_recordings.

Ask for programme information in plain language, for example:

- What is on ARD tonight?
- Find upcoming Formula 1 broadcasts.
- When is the next episode of Tatort?
- Search for films airing this weekend.
- What children’s programmes are on tomorrow morning?
- Find recordings of Planet Earth.
- Search my recordings for anything from ARD.

I’ll translate the request into an EPG or recording search and return matching programme or recording details. It cannot change recordings, timers, or VDR configuration.
```

### Claude Code

Add a local stdio server in the current project:

```bash
claude mcp add --transport stdio vdradmin -- \
  /usr/bin/vdradmin-go-mcp --config /var/lib/vdradmin-go/config.yaml
```

Add the HTTP service instead:

```bash
claude mcp add --transport http vdradmin http://127.0.0.1:8081/mcp
```

Use `--scope user` for a server available to all projects, or `--scope project` to create a shareable `.mcp.json`. Run `claude mcp get vdradmin` or `/mcp` to verify connectivity.

### Codex

Add a local stdio server:

```bash
codex mcp add vdradmin -- \
  /usr/bin/vdradmin-go-mcp --config /var/lib/vdradmin-go/config.yaml
```

Alternatively, add one of these entries to `~/.codex/config.toml` or a trusted project's `.codex/config.toml`:

```toml
[mcp_servers.vdradmin]
command = "/usr/bin/vdradmin-go-mcp"
args = ["--config", "/var/lib/vdradmin-go/config.yaml"]
```

```toml
[mcp_servers.vdradmin]
url = "http://127.0.0.1:8081/mcp"
```

Run `codex mcp list` or use `/mcp` in the Codex terminal UI to inspect the server.

## Troubleshooting

- Confirm VDR is reachable with the configured `vdr.host` and `vdr.port`.
- Check service logs with `journalctl -u vdradmin-go-mcp -e`.
- For stdio failures, inspect the MCP client log; standard output is protocol data and should remain silent outside MCP responses.
- Use [MCP Inspector](https://github.com/modelcontextprotocol/inspector) to initialize the server, list tools, and call `search_epg` during development.
- A zero-result search is valid; first verify the configured `vdr.wanted_channels` has not excluded the expected channel.

## References

- [MCP server guide](https://modelcontextprotocol.io/docs/develop/build-server)
- [VS Code MCP servers](https://code.visualstudio.com/docs/copilot/chat/mcp-servers)
- [Claude Code MCP](https://code.claude.com/docs/en/mcp)
- [Codex MCP](https://learn.chatgpt.com/docs/extend/mcp)
