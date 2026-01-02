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

## Configuration

See `configs/config.example.yaml` for full configuration options.

## License

LGPL v2.1 (same as original [vdradmin-am](http://andreas.vdr-developer.org/vdradmin-am/index.html))
