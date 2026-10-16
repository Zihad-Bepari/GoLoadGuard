# Contributing to GoLoadGuard

Thank you for your interest in contributing to GoLoadGuard! We welcome contributions that maintain our high standards for correctness, simplicity, and performance.

## Design Philosophy

1. **Zero External Dependencies**: The core library must only depend on the Go standard library (`sync`, `context`, etc.).
2. **Correctness Over Features**: We prioritize deterministic concurrency semantics, leak-free permit lifecycle, and race safety over adding complex or experimental features.
3. **Evidence-Based Changes**: Every performance optimization or behavioral modification must be accompanied by reproducible tests and benchmarks.

## Development Workflow

1. Fork the repository and create your feature branch:
   ```bash
   git checkout -b feature/my-feature
   ```

2. Format code and run static checks:
   ```bash
   gofmt -w .
   go vet ./...
   ```

3. Run unit tests and the race detector:
   ```bash
   go test -v -race -count=1 ./...
   ```

4. Run stress tests:
   ```bash
   go test -race -count=10 ./...
   ```

5. Run benchmarks:
   ```bash
   go test -run=^$ -bench=. -benchmem ./...
   ```

6. Submit a Pull Request with a clear description of the problem solved, design trade-offs, and test evidence.
