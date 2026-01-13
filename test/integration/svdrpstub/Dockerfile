FROM golang:1.25-alpine AS builder

WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/svdrp-stub ./

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
EXPOSE 6419
COPY --from=builder /out/svdrp-stub /svdrp-stub
ENTRYPOINT ["/svdrp-stub"]
