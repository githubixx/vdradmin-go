# Multi-stage build for smaller image size
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-w -s" -o vdradmin ./cmd/vdradmin

# Final stage - minimal runtime image
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/vdradmin .

# Copy web assets
COPY --from=builder /build/web ./web

# Copy example config
COPY --from=builder /build/configs/config.example.yaml ./config.example.yaml

# Create non-root user
RUN addgroup -g 1000 vdradmin && \
    adduser -D -u 1000 -G vdradmin vdradmin && \
    chown -R vdradmin:vdradmin /app

USER vdradmin

EXPOSE 8080

ENTRYPOINT ["/app/vdradmin"]
CMD ["--config", "/app/config.yaml"]
