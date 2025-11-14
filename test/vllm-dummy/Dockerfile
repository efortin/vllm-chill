FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY main.go .

# Build static binary
RUN CGO_ENABLED=0 go build -o vllm-dummy main.go

FROM scratch
COPY --from=builder /app/vllm-dummy /vllm-dummy

ENV MODEL_NAME="test/model" \
    SERVED_MODEL_NAME="test-model" \
    PORT="8000"

EXPOSE 8000
ENTRYPOINT ["/vllm-dummy"]
