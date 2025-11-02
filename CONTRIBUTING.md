# Contributing to vLLM AutoScaler

Thank you for your interest in contributing to vLLM AutoScaler!

## Development Setup

1. **Prerequisites**
   - Go 1.23 or later
   - Docker with buildx support
   - [Task](https://taskfile.dev/) (optional)
   - Access to a Kubernetes cluster (for testing)

2. **Clone the repository**
   ```bash
   git clone https://github.com/yourusername/vllm-autoscaler.git
   cd vllm-autoscaler
   ```

3. **Install dependencies**
   ```bash
   go mod download
   ```

4. **Run tests**
   ```bash
   task test
   # or
   go test -v ./...
   ```

## Development Workflow

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Add tests for your changes
5. Run tests (`task test`)
6. Commit your changes (`git commit -m 'Add amazing feature'`)
7. Push to your branch (`git push origin feature/amazing-feature`)
8. Open a Pull Request

## Running Locally

```bash
# Run the autoscaler locally (requires kubeconfig)
task run

# Or with custom flags
go run ./cmd/autoscaler serve --namespace my-namespace --idle-timeout 10m
```

## Code Guidelines

- Follow Go best practices and idioms
- Add tests for new features
- Update documentation as needed
- Keep commits atomic and well-described
- Run `go fmt` before committing
- Ensure all tests pass

## Testing

```bash
# Run all tests
task test

# Run tests with coverage
go test -v -race -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Pull Request Process

1. Update the README.md with details of changes if needed
2. Update the documentation in `docs/` if needed
3. Add tests for your changes
4. Ensure all CI checks pass
5. Request review from maintainers

## Questions?

Feel free to open an issue for any questions or concerns!
