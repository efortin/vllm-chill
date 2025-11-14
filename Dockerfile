# Build stage
FROM golang:1.24-bookworm AS builder

# Install build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    libc6-dev \
    && rm -rf /var/lib/apt/lists/*

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

# Build with CGO enabled for NVML support
RUN CGO_ENABLED=1 go build \
    -ldflags="-w -s -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o vllm-chill ./cmd/autoscaler

# Final stage - use NVIDIA base for runtime NVML access
FROM nvidia/cuda:12.1.0-base-ubuntu22.04

# Install ca-certificates
RUN apt-get update && apt-get install -y \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Copy binary from builder
COPY --from=builder /app/vllm-chill /vllm-chill

EXPOSE 8080

ENTRYPOINT ["/vllm-chill"]
CMD ["serve"]
