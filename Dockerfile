# Build stage with CUDA support for NVML
FROM nvidia/cuda:12.1.0-base-ubuntu22.04 AS builder

# Install Go and build dependencies
RUN apt-get update && apt-get install -y \
    wget \
    git \
    gcc \
    libc6-dev \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Install Go 1.25
RUN wget https://go.dev/dl/go1.23.4.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf go1.23.4.linux-amd64.tar.gz && \
    rm go1.23.4.linux-amd64.tar.gz

ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOPATH="/go"
ENV PATH="${GOPATH}/bin:${PATH}"

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build with CGO enabled for NVML support
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build \
    -a \
    -ldflags="-w -s" \
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
