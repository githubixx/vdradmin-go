# Multi-stage build for smaller image size
ARG APP_BUILDER=builder
FROM golang:1.26-alpine AS source

# Install build dependencies
RUN apk add --no-cache git make

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Copy source code
COPY . .

# Build the application
FROM source AS builder
RUN sh scripts/check-release-readiness.sh && go mod download && \
    CGO_ENABLED=0 GOOS=linux go build -mod=readonly -a -installsuffix cgo -ldflags="-w -s" -o vdradmin-go ./cmd/vdradmin-go

# Opt-in local Chroma workspace, supplied as the chroma-ui named build context.
FROM source AS builder-workspace
COPY --from=chroma-ui go.mod go.sum /chroma-ui/
COPY --from=chroma-ui *.go /chroma-ui/
COPY --from=chroma-ui assets /chroma-ui/assets
COPY --from=chroma-ui templates /chroma-ui/templates
RUN go work init /build /chroma-ui && \
    CGO_ENABLED=0 GOOS=linux go build -mod=readonly -a -installsuffix cgo -ldflags="-w -s" -o vdradmin-go ./cmd/vdradmin-go

FROM ${APP_BUILDER} AS selected

# Final stage - minimal runtime image
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=selected /build/vdradmin-go .

# VDR assets remain filesystem-backed; Chroma UI assets are embedded.
COPY --from=selected /build/web/templates ./web/templates
COPY --from=selected /build/web/static ./web/static

# Copy example config
COPY --from=selected /build/configs/config.example.yaml ./config.example.yaml

# Create non-root user
RUN addgroup -g 1000 vdradmin && \
    adduser -D -u 1000 -G vdradmin vdradmin && \
    chown -R vdradmin:vdradmin /app

USER vdradmin

EXPOSE 8080

ENTRYPOINT ["/app/vdradmin-go"]
CMD ["--config", "/app/config.yaml"]
