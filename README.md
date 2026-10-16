# GoLoadGuard — Universal In-Process Concurrency & Load Protection Library

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Platforms](https://img.shields.io/badge/Platforms-Go%20%7C%20Node.js%20%7C%20NestJS%20%7C%20PHP%20%7C%20Laravel-brightgreen.svg)](#supported-platforms)
[![Zero-Dependency](https://img.shields.io/badge/Zero--Dependency-Core-orange.svg)](#architecture-philosophy)
[![Hosting Friendly](https://img.shields.io/badge/cPanel%20%26%20Shared%20Hosting-100%25%20Compatible-success.svg)](#why-cpanel--shared-hosting-compatible)

Production-grade, zero-dependency, in-process concurrency control, bounded FIFO queuing, and backpressure protection library designed to run universally across **Go, Node.js, NestJS, PHP, and Laravel** with **100% Shared Hosting and cPanel compatibility**.

---

## 📑 Table of Contents
1. [Executive Summary: What is GoLoadGuard?](#-executive-summary-what-is-goloadguard)
2. [The Core Problem: Goodput Collapse & OOM Kills](#-the-core-problem-goodput-collapse--oom-kills)
3. [How GoLoadGuard Works: Two-Tier Admission Gate](#-how-goloadguard-works-two-tier-admission-gate)
4. [Competitive Comparison: Why GoLoadGuard is Better](#-competitive-comparison-why-goloadguard-is-better)
5. [The Engineering Breakthrough: 100% cPanel & Shared Hosting Support](#-the-engineering-breakthrough-100-cpanel--shared-hosting-support)
6. [Supported Platforms & Installation](#-supported-platforms--installation)
7. [Language-by-Language Integration Guide](#-language-by-language-integration-guide)
   - [1. Go (Golang)](#1-go-golang)
   - [2. Node.js (Express / Fastify)](#2-nodejs-express--fastify)
   - [3. NestJS](#3-nestjs)
   - [4. PHP (Native / Pure)](#4-php-native--pure)
   - [5. Laravel (Framework)](#5-laravel-framework)
8. [Configuration Reference](#-configuration-reference)
9. [Observability & Metrics](#-observability--metrics)
10. [Testing & Verification](#-testing--verification)
11. [License & Contributing](#-license--contributing)

---

## 🚀 Executive Summary: What is GoLoadGuard?

**GoLoadGuard** is an open-source, multi-runtime concurrency management and backpressure library. Unlike traditional rate limiters that merely count requests over time, GoLoadGuard controls **simultaneous in-flight executions** to shield critical server resources (CPU, RAM, database connection pools, file descriptors) from exhaustion.

### Core Design Principles:
- **Zero External Infrastructure**: No Redis, Memcached, Consul, or Docker containers required.
- **In-Process Execution**: Operates directly inside the application runtime for sub-millisecond execution.
- **Deterministic Bounded Queuing**: Enforces strict FIFO queue ordering with guaranteed capacity caps.
- **Strict Anti-OOM Guarantee**: Drops excess requests immediately (`503 Service Unavailable` / `429 Too Many Requests`) before server memory spikes.

---

## 💥 The Core Problem: Goodput Collapse & OOM Kills

Under sudden traffic bursts or downstream latencies (e.g., slow SQL queries, third-party API lag), web servers (such as Go `net/http`, Node.js Event Loop, or PHP-FPM) attempt to accept every incoming connection.

```
[Normal Load]     Traffic: 100 req/s  ──► Latency: 50ms   ──► Goodput: 100 req/s (100% Successful)
[Overload Load]   Traffic: 2000 req/s ──► Latency: 8000ms ──► Goodput: 0 req/s   (TOTAL COLLAPSE!)
```

### Why does this cause total outage?
1. **Unbounded Goroutines / Event Loop Lag / FPM Worker Bloat**: Memory usage grows exponentially as thousands of stalled requests accumulate.
2. **Cascading Timeouts**: Callers configure client-side timeouts (e.g., 3 seconds). If a request spends 4 seconds waiting in an internal queue before execution starts, the server burns CPU and database resources completing work for an already-disconnected client.
3. **Goodput Collapse**: The server operates at 100% CPU and RAM utilization, yet **delivers zero usable responses** to users.
4. **Linux OOM-Killer & cPanel 508 Errors**: On VPS/Containers, the Linux kernel terminates the process via Out-Of-Memory kill. On shared hosting, CloudLinux triggers `508 Resource Limit Reached` due to Entry Process (EP) exhaustion.

**GoLoadGuard stops this death spiral** by shedding excess workload at the gate, keeping active work at an optimal latency band.

---

## ⚙️ How GoLoadGuard Works: Two-Tier Admission Gate

GoLoadGuard deploys a two-tier admission control gate:

```
Incoming Request
      │
      ▼
[ Are Active Slots < MaxConcurrency ? ] ──Yes──► [ Execute Work Immediately ]
      │ No
      ▼
[ Is Queue Depth < MaxQueueSize ? ]      ──Yes──► [ Wait in Strict FIFO Queue ]
      │ No                                             │
      ▼                                                ├── Slot Freed ──► [ Execute Work ]
[ Fast Rejection (503 Service Unavailable) ]          └── Timeout/Abort ──► [ Safe Cleanup ]
```

1. **Tier 1 (Execution Cap)**: Up to `MaxConcurrency` requests run in parallel.
2. **Tier 2 (Bounded FIFO Queue)**: Up to `MaxQueueSize` callers wait in exact order of arrival.
3. **Immediate Fast-Fail**: Once the queue is saturated, incoming requests are rejected immediately without allocating heap objects or spawning goroutines/subprocesses.
4. **Post-Grant Cancellation Contract**: If a caller's context aborts right as a slot is granted, the user callback is skipped and the permit is safely passed to the next waiting caller—preventing permit leakage.

---

## 📊 Competitive Comparison: Why GoLoadGuard is Better

| Criteria | Go `x/sync/semaphore` | Token Bucket / `rate` | Netflix Concurrency Limits | **GoLoadGuard** |
| :--- | :--- | :--- | :--- | :--- |
| **Bounded Queue Capacity** | ❌ No (Unbounded) | ❌ No | ⚠️ Complex / Heuristic | ✅ **Strict Bounded $O(1)$ FIFO** |
| **Anti-OOM Protection** | ❌ No | ❌ No | ⚠️ High Memory Footprint | ✅ **Guaranteed Memory Cap** |
| **Zero External Dependencies**| ✅ Yes | ✅ Yes | ❌ Heavy dependencies | ✅ **Zero Dependencies (Stdlib)**|
| **Post-Grant Cancellation Race**| ❌ Prone to permit leaks| N/A | ❌ Complex race conditions | ✅ **Deterministic Mutex State** |
| **Multi-Language Support** | ❌ Go Only | ❌ Go Only | ❌ Go / Java Only | ✅ **Go, Node, Nest, PHP, Laravel** |
| **Shared Hosting Compatibility**| ❌ Daemon required | ❌ In-memory only | ❌ Not supported | ✅ **100% cPanel / Shared Hosting** |

---

## 🛡️ The Engineering Breakthrough: 100% cPanel & Shared Hosting Support

Running concurrency control in shared hosting environments (cPanel, CloudLinux, LiteSpeed, Apache mod_php) has traditionally been impossible because:
1. `disable_functions` in `php.ini` globally blocks `exec()`, `shell_exec()`, and `proc_open()`.
2. CloudLinux kills persistent background daemons and caps Entry Processes (EP) to 20–30.
3. Shared environments lack Redis, Memcached, or root permissions.

### How GoLoadGuard Solves This:
- **Zero Daemon**: No persistent background services running.
- **Zero Subprocess**: No calls to `exec()` or external binaries.
- **Kernel-Level Atomic Locks (`flock`)**: Uses native OS file locking on temporary files (`/tmp` or `storage/framework/`) with non-blocking evaluation (`LOCK_EX | LOCK_NB`).
- **Application Directory Hash Scoping**: Generates isolated lock namespaces per application directory, preventing cross-tenant collisions on multi-tenant shared servers.
- **Automatic Kernel Cleanup**: When a PHP process terminates (even upon crash or fatal error), the Linux kernel automatically unbinds file locks, preventing system lockouts.

---

## 📦 Supported Platforms & Installation

| Platform | Package Manager | Package Identifier | Hosting Targets |
| :--- | :--- | :--- | :--- |
| **Go** | `go get` | `github.com/goloadguard/goloadguard` | VPS, Docker, K8s, Cloud |
| **Node.js** | `npm install` | `@goloadguard/node` | cPanel Node.js App, Vercel, VPS |
| **NestJS** | `npm install` | `@goloadguard/node` | Cloud, Microservices, VPS |
| **PHP** | `composer require`| `goloadguard/goloadguard` | cPanel, Shared Hosting, VPS |
| **Laravel** | `composer require`| `goloadguard/goloadguard` | Shared Hosting, Laravel Octane, Forge |

---

## 💻 Language-by-Language Integration Guide

---

### 1. Go (Golang)

Core package implemented using Go standard library (`sync.Mutex` and an explicit doubly-linked list).

#### Installation
```bash
go get github.com/goloadguard/goloadguard
```

#### Core Usage (`Do`)
```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/goloadguard/goloadguard"
)

func main() {
	// Max 10 concurrent operations, queue depth of 20
	guard, err := goloadguard.New(goloadguard.Config{
		MaxConcurrency: 10,
		MaxQueueSize:   20,
	})
	if err != nil {
		panic(err)
	}
	defer guard.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Protected critical section
	err = guard.Do(ctx, func(ctx context.Context) error {
		// Database query or CPU-bound computation
		time.Sleep(50 * time.Millisecond)
		return nil
	})

	if err != nil {
		fmt.Printf("Workload rejected or canceled: %v\n", err)
	}
}
```

#### HTTP Middleware (`httpguard`)
```go
package main

import (
	"net/http"
	"github.com/goloadguard/goloadguard"
	"github.com/goloadguard/goloadguard/httpguard"
)

func main() {
	guard, _ := goloadguard.New(goloadguard.Config{
		MaxConcurrency: 50,
		MaxQueueSize:   100,
	})
	defer guard.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/process", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Protected payload"))
	})

	// Wrap handler with automated 503 and Retry-After headers
	protectedHandler := httpguard.Middleware(guard, httpguard.Options{
		OverloadStatusCode: http.StatusServiceUnavailable, // 503
		RetryAfterSeconds:  5,                           // Retry-After: 5
	})(mux)

	http.ListenAndServe(":8080", protectedHandler)
}
```

---

### 2. Node.js (Express / Fastify)

Pure JavaScript/TypeScript in-process event-queue implementation with zero native C++ compiler dependencies.

#### Installation
```bash
npm install @goloadguard/node
```

#### Express Middleware Usage
```typescript
import express from 'express';
import { LoadGuard, expressMiddleware } from '@goloadguard/node';

const app = express();

// Configure 15 concurrent slots, 30 queued callers, 2s queue timeout
const guard = new LoadGuard({
  maxConcurrency: 15,
  maxQueueSize: 30,
  queueTimeoutMs: 2000,
});

// Register global or route-specific middleware
app.use(expressMiddleware(guard, {
  overloadStatus: 503,
  retryAfterSeconds: 5,
}));

app.get('/api/heavy-workload', async (req, res) => {
  res.send({ status: 'success', message: 'Executed safely under concurrency cap' });
});

app.listen(3000, () => console.log('Server running on port 3000'));
```

---

### 3. NestJS

Idiomatic NestJS `@Injectable() CanActivate` Guard integration with automatic HTTP status formatting.

#### Installation
```bash
npm install @goloadguard/node
```

#### Controller Usage
```typescript
import { Controller, Get, UseGuards } from '@nestjs/common';
import { LoadGuard, NestLoadGuard } from '@goloadguard/node';

// Instantiate guard instance
const reportsGuard = new LoadGuard({ maxConcurrency: 5, maxQueueSize: 10 });

@Controller('reports')
export class ReportController {

  @Get('financial-export')
  @UseGuards(new NestLoadGuard(reportsGuard, 503))
  async exportReport() {
    return { data: 'Export completed safely without server degradation' };
  }
}
```

---

### 4. PHP (Native / Pure)

cPanel and Shared-Hosting ready in-process concurrency guard using atomic OS kernel file locks (`flock`).

#### Installation
```bash
composer require goloadguard/goloadguard
```

#### Native PHP Usage
```php
<?php

require_once __DIR__ . '/vendor/autoload.php';

use GoLoadGuard\Guard;
use GoLoadGuard\OverloadedException;
use GoLoadGuard\QueueTimeoutException;

// 10 concurrent slots, 20 waiting callers, 3000ms queue timeout
$guard = new Guard(
    maxConcurrency: 10,
    maxQueueSize: 20,
    queueTimeoutMs: 3000
);

try {
    $result = $guard->run(function () {
        // Critical Section (e.g., PDF generation, database migration, heavy compute)
        usleep(50000); // 50ms simulated load
        return "Operation Successful";
    });

    echo $result;
} catch (OverloadedException $e) {
    http_response_code(503);
    header('Retry-After: 5');
    echo json_encode(['error' => 'Service Unavailable: Server Overloaded']);
} catch (QueueTimeoutException $e) {
    http_response_code(504);
    echo json_encode(['error' => 'Gateway Timeout: Queue Wait Exceeded']);
}
```

---

### 5. Laravel (Framework)

Includes auto-registered ServiceProvider, Route Middleware, and storage directory auto-detection.

#### Installation
```bash
composer require goloadguard/goloadguard
```

#### Route Protection (`routes/api.php` or `routes/web.php`)
The `loadguard` middleware alias is automatically discovered:

```php
use App\Http\Controllers\CheckoutController;
use App\Http\Controllers\ExportController;
use Illuminate\Support\Facades\Route;

// Parameters: maxConcurrency, maxQueueSize, queueTimeoutMs, retryAfterSeconds
Route::middleware(['loadguard:20,50,3000,5'])->group(function () {
    Route::post('/checkout', [CheckoutController::class, 'process']);
    Route::get('/analytics/export', [ExportController::class, 'download']);
});
```
*When overloaded, Laravel automatically responds with HTTP `503 Service Unavailable` and a `Retry-After: 5` header without crashing your shared hosting Entry Process limits.*

---

## ⚙️ Configuration Reference

| Parameter | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `maxConcurrency` | `int` | **Required (> 0)** | Maximum number of concurrent operations allowed simultaneously. |
| `maxQueueSize` | `int` | `0` | Maximum queue depth for waiting callers. `0` disables queuing (instant fast-fail). |
| `queueTimeoutMs` | `int` | `0` | Maximum duration (ms) a caller can wait in queue. `0` disables timeout. |
| `retryAfterSeconds` | `int` | `5` | `Retry-After` HTTP header value sent back on overload rejections. |

---

## 📈 Observability & Metrics

For runtime introspection and health monitoring, GoLoadGuard provides instantaneous snapshots:

```go
stats := guard.Stats()
fmt.Printf("Active: %d | Queue: %d | Accepted: %d | Rejected: %d | Canceled: %d\n",
	stats.CurrentInFlight,
	stats.CurrentQueueSize,
	stats.AcceptedRequests,
	stats.RejectedRequests,
	stats.CanceledRequests,
)
```

---

## 🧪 Testing & Verification

Run the test suite across supported runtimes:

### Go (Unit & Race Detector)
```bash
go test -v -race -count=1 ./...
go test -run=^$ -bench=. -benchmem ./...
```

### Node.js (Live Test Runner)
```bash
cd packages/node
node test_runner.js
```

### PHP (Syntax Validation)
```bash
php -l packages/php/src/Guard.php
php -l packages/php/src/Laravel/LoadGuardMiddleware.php
```

---

## 📄 License & Contributing

Distributed under the **MIT License**. See [LICENSE](LICENSE) for details.

Contributions, issues, and feature requests are welcome! Check out [CONTRIBUTING.md](CONTRIBUTING.md) to get started.
