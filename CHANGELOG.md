# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Core `Guard` with fixed concurrency control (`MaxConcurrency`) and bounded FIFO waiting queue (`MaxQueueSize`).
- Deterministic single linearization point for grant-versus-cancellation races under mutex synchronization.
- Post-grant cancellation validation preventing callback execution on stale grants and guaranteeing permit handoff.
- Zero external runtime dependencies (pure Go standard library).
- Idempotent and non-blocking `Close()` unblocking queued waiters with `ErrClosed`.
- Point-in-time consistent metrics snapshot (`Stats`).
- Drop-in HTTP middleware (`httpguard`) with configurable `503 Service Unavailable` / `429 Too Many Requests` status codes and `Retry-After` headers.
- Comprehensive unit, stress, cancellation, and concurrency test suites with race detection.
- Complete benchmarks for uncontended and parallel contention scenarios.
- Realistic runnable examples for HTTP servers and background worker pools.
