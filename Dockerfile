# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build args for versioning (optional - defaults for manual builds)
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

# Build static binary with CGO disabled
RUN CGO_ENABLED=0 go build \
    -ldflags="-w -s -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o vllm-chill ./cmd/autoscaler

# Final stage - use minimal Alpine base
FROM alpine:latest

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

# Copy binary from builder
COPY --from=builder /app/vllm-chill /vllm-chill

EXPOSE 8080

ENTRYPOINT ["/vllm-chill"]
CMD ["serve"]
