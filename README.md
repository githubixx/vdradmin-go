# vdradmin-go

Modern rewrite of [vdradmin-am](http://andreas.vdr-developer.org/vdradmin-am/index.html) in Go with hexagonal architecture, clean code practices, and modern web technologies. **Currently this is only a prove of concept and it will eat your computer!** ;-)

## IMPORTANT NOTE

This version is usable in some regards. **Nevertheless if you run it and it destroys your computer, deletes your videos or something like that, it's YOUR problem!** ;-) **Test it only** with a test [VDR](https://tvdr.de/) instance!

This code was mainly generated with Claude Code and GPT-5.2. I just wanted to see how far I can get converting the quite dated code base of [vdradmin-am](http://andreas.vdr-developer.org/vdradmin-am/index.html) to Go and more recent technologies.

For now the code compiles and it displays something. That's it! **DO NOT expect to get something close to vdradmin-am!!!**

## Screenshots

![Alt text](screenshots/vdradmin-go_channels.png)

## Goals

- **Modern Architecture**: Hexagonal (ports & adapters) architecture for maintainability
- **Clean Code**: Following Go best practices and SOLID principles
- **Modern UI**: [htmx](https://htmx.org/) for dynamic interactions, modern CSS, minimal JavaScript
- **Type Safety**: Strong typing with comprehensive error handling
- **Performance**: Concurrent operations, efficient caching
- **Security**: Secure authentication, input validation, HTTPS support
- **Observability**: Structured logging, metrics, tracing ready

## Architecture

```plain
vdradmin-go/
├── build/                   # Build output (make build)
├── cmd/
│   └── vdradmin/            # Application entry point
├── internal/
│   ├── domain/              # Core business logic (entities, value objects)
│   ├── ports/               # Interfaces (primary & secondary ports)
│   ├── adapters/            # Implementations
│   │   ├── primary/http/    # Incoming HTTP server, handlers, middleware
│   │   └── secondary/svdrp/ # Outgoing VDR integration (SVDRP client)
│   ├── application/         # Use cases, services
│   └── infrastructure/      # Cross-cutting concerns (logging, config)
├── web/
│   ├── templates/           # HTML templates
│   └── static/              # CSS + minimal JS
├── configs/                 # Configuration files
├── deployments/             # Docker + systemd service
├── docs/                    # Documentation (see docs/ARCHITECTURE.md)
└── screenshots/             # UI screenshots
```

## Technology Stack

- **Language**: Go 1.23+
- **Web**: Go 1.22+ internal router
- **Templates**: html/template
- **Config**: YAML with validation
- **Logging**: slog (stdlib)
- **Frontend**: htmx + modern CSS

## Quick Start

```bash
# Build
make build

# Run
./build/vdradmin --config config.yaml
```

## Usage / Deployment Options

vdradmin-go can be run in multiple ways depending on your setup. Releases are built automatically via GitHub Actions when a new release tag is created.

### 1) Use the released binary (recommended)

1. Download the `linux_amd64` archive from the latest GitHub Release.
2. Extract it and run:

```bash
./vdradmin --config /path/to/config.yaml
```

Tip: `./vdradmin --version` prints the release version.

### 2) Use the Docker container (GHCR)

Pull and run the image from GitHub Container Registry:

```bash
docker pull ghcr.io/<owner>/<repo>:1.2.3
docker run \
  --rm -p 8080:8080 \
  -v "${PWD}/config.yaml:/app/config.yaml:ro" \
  ghcr.io/<owner>/<repo>:1.2.3
```

### 3) Run as a systemd service

If you want vdradmin-go to start automatically on boot:

1. Copy the example unit file from `deployments/systemd/vdradmin.service` to your systemd directory.
2. Install the `vdradmin` binary somewhere like `/usr/local/bin/vdradmin`.
3. Ensure the unit points to your config path.
4. Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now vdradmin
```

### 4) Use docker-compose

There is an example compose file at `deployments/docker-compose.yml`.
Typically you will:

- set the image to `ghcr.io/<owner>/<repo>:1.2.3`
- mount your `config.yaml` into the container

Then run:

```bash
docker compose -f deployments/docker-compose.yml up -d
```

## Configuration

See `configs/config.example.yaml` for full configuration options.

## Watch TV

The **Watch TV** page (`/watch`) provides:

- a periodically refreshed **snapshot** (configurable interval + size)
- a glossy **remote control** (SVDRP `HITK`)
- a **channel list** restricted to channels configured in **Configurations** (wanted channels)

### Requirements

The snapshot feature uses the SVDRP `GRAB` command.

- If your VDR does not support `GRAB` (or cannot grab a picture on the VDR host), the page will still load but snapshots will fail. This is common for headless/recording-only VDR instances that have tuners but no primary video output/decoder device.

For troubleshooting and background, see `docs/WATCHTV.md`.

### Optional: stream URL mode (headless setups)

If you run VDR headless (recording-only) and `GRAB` cannot work, you can enable streaming for `/watch`.

**Recommended: HLS proxy (built-in transcoding)**

Configure `vdr.streamdev_backend_url` in Configurations:
- Example: `http://127.0.0.1:3000/{channel}`
- Requires: `ffmpeg` installed on the vdradmin-go host
- vdradmin-go will transcode streamdev MPEG-TS to browser-playable HLS automatically
- `/watch` uses internal proxy endpoint `/watch/stream/{channel}/index.m3u8`

**Alternative: Direct external stream URL**

- Configure `vdr.stream_url_template` if you have a pre-existing HLS/MJPEG/WebM endpoint
- The template may contain `{channel}`, which will be replaced with the selected VDR channel **number** (e.g. `1`, `2`, `3`).
- `/watch` embeds the URL into an HTML5 `<video>` element. The URL must point to a format your browser can play (for example HLS `.m3u8` or a browser-supported MP4/WebM stream).

If you use `vdr-plugin-streamdev-server`, its HTTP server commonly runs on port `3000` and can serve channels by number, e.g. `http://127.0.0.1:3000/1`.
Note: streamdev’s default outputs are typically TS/PES/ES, which most browsers do not play directly; you may need an external remux/transcode step to get true in-browser playback.

## Integration Tests (Docker)

There is an optional integration test (build tag `integration`) that spins up:

- an SVDRP stub container (fake VDR)
- a `vdradmin-go` container (to mimic real deployment)

and asserts that `/timers` renders the timer timeline with `ok`/`collision`/`critical` blocks.

```bash
# Requires Docker
go test -tags=integration ./internal/integration -run TestContainers -count=1

# Optional: reuse a prebuilt app image (faster)
docker build -f deployments/Dockerfile -t vdradmin-go-it-app:local .
VDRADMIN_GO_APP_IMAGE=vdradmin-go-it-app:local \
	go test -tags=integration ./internal/integration -run TestContainers -count=1
```

## License

LGPL v2.1 (same as original [vdradmin-am](http://andreas.vdr-developer.org/vdradmin-am/index.html))
