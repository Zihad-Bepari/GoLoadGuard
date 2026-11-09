# GoLoadGuard — Project Setup & Integration Guide

This guide provides end-to-end instructions for installing, configuring, and integrating **GoLoadGuard** across all supported environments: **Go (Golang)**, **Node.js (Express & Fastify)**, **NestJS**, **PHP (Pure/Native)**, and **Laravel**.

---

## 📑 Table of Contents
1. [Supported Platforms & Installation Summary](#supported-platforms--installation-summary)
2. [1. Go (Golang) Setup](#1-go-golang-setup)
   - [Installation](#go-installation)
   - [Core Workload Guarding (`Do`)](#core-workload-guarding-do)
   - [HTTP Server Middleware (`httpguard`)](#http-server-middleware-httpguard)
   - [Integration with Gin, Chi, and Fiber](#integration-with-gin-chi-and-fiber)
   - [Runtime Observability & Metrics](#runtime-observability--metrics)
3. [2. Node.js & Express Setup](#2-nodejs--express-setup)
   - [Installation](#nodejs-installation)
   - [1-Line Plug-and-Play Middleware](#1-line-plug-and-play-middleware)
   - [Custom Instance & Route-Specific Guarding](#custom-instance--route-specific-guarding)
   - [Zero-Leak Socket Protection Verification](#zero-leak-socket-protection-verification)
4. [3. NestJS Setup](#3-nestjs-setup)
   - [Global & Controller Guard Registration](#global--controller-guard-registration)
5. [4. PHP (Native / Pure) Setup](#4-php-native--pure-setup)
   - [Installation](#php-installation)
   - [Core Closure Usage (`run`)](#core-closure-usage-run)
   - [Manual Acquire & Release Pattern](#manual-acquire--release-pattern)
6. [5. Laravel Framework Setup](#5-laravel-framework-setup)
   - [Service Provider & Middleware Registration](#service-provider--middleware-registration)
   - [Route Protection in Laravel 10 & Laravel 11](#route-protection-in-laravel-10--laravel-11)
7. [Configuration Reference](#configuration-reference)
8. [Production Deployment & Tuning](#production-deployment--tuning)
   - [cPanel & CloudLinux Shared Hosting](#cpanel--cloudlinux-shared-hosting)
   - [Docker & Containerized Microservices](#docker--containerized-microservices)
   - [Linux VPS & Nginx Reverse Proxy](#linux-vps--nginx-reverse-proxy)

---

## Supported Platforms & Installation Summary

| Platform | Package Manager | Package Identifier / Direct Install | Minimum Runtime |
| :--- | :--- | :--- | :--- |
| **Go** | `go get` | `github.com/Zihad-Bepari/GoLoadGuard` | Go 1.21+ |
| **Node.js** | `npm` / `pnpm` / `yarn` | `@goloadguard/node` *(or `github:Zihad-Bepari/GoLoadGuard`)* | Node.js 18+ |
| **NestJS** | `npm` | `@goloadguard/node` | NestJS 9+ |
| **PHP** | `composer` | Direct VCS or local path repository | PHP 8.0+ |
| **Laravel** | `composer` | Direct VCS or local path repository | Laravel 9, 10, 11 |

---

## 1. Go (Golang) Setup

### Go Installation

Add GoLoadGuard to your project's `go.mod`:

```bash
go get github.com/Zihad-Bepari/GoLoadGuard
```

### Core Workload Guarding (`Do`)

Wrap any critical computation, database transaction, or external API call inside `guard.Do()`:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Zihad-Bepari/GoLoadGuard"
)

func main() {
	// Initialize guard: Max 20 concurrent tasks, queue capacity of 50
	guard, err := goloadguard.New(goloadguard.Config{
		MaxConcurrency: 20,
		MaxQueueSize:   50,
	})
	if err != nil {
		log.Fatalf("failed to initialize GoLoadGuard: %v", err)
	}
	defer guard.Close()

	// Create a context with an overall timeout for waiting in queue + executing
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Execute protected operation
	err = guard.Do(ctx, func(execCtx context.Context) error {
		// Expensive database query or background task
		time.Sleep(100 * time.Millisecond)
		return nil
	})

	if err != nil {
		switch {
		case err == goloadguard.ErrOverloaded:
			fmt.Println("503: System overloaded, capacity and queue are both full.")
		case err == goloadguard.ErrClosed:
			fmt.Println("Guard has been closed during application shutdown.")
		case err == context.DeadlineExceeded:
			fmt.Println("Operation timed out while waiting in queue or during execution.")
		default:
			fmt.Printf("Operation returned error: %v\n", err)
		}
	}
}
```

### HTTP Server Middleware (`httpguard`)

Protect your standard Go HTTP server with automated `503 Service Unavailable` responses and `Retry-After` headers:

```go
package main

import (
	"net/http"
	"time"

	"github.com/Zihad-Bepari/GoLoadGuard"
	"github.com/Zihad-Bepari/GoLoadGuard/httpguard"
)

func main() {
	// Guard: 100 concurrent requests, 200 waiting queue
	guard, _ := goloadguard.New(goloadguard.Config{
		MaxConcurrency: 100,
		MaxQueueSize:   200,
	})
	defer guard.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"success"}`))
	})

	// Wrap handler with httpguard middleware
	protectedMux := httpguard.Middleware(guard, httpguard.Options{
		OverloadStatusCode: http.StatusServiceUnavailable, // 503
		RetryAfterSeconds:  5,                           // Retry-After: 5
	})(mux)

	server := &http.Server{
		Addr:    ":8080",
		Handler: protectedMux,
	}
	server.ListenAndServe()
}
```

### Integration with Gin, Chi, and Fiber

#### Gin Web Framework
```go
func GinLoadGuard(guard *goloadguard.Guard) gin.HandlerFunc {
	return func(c *gin.Context) {
		err := guard.Do(c.Request.Context(), func(ctx context.Context) error {
			c.Next()
			return nil
		})
		if err != nil {
			if errors.Is(err, goloadguard.ErrOverloaded) {
				c.Header("Retry-After", "5")
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "Server Overloaded"})
				return
			}
			c.AbortWithStatus(http.StatusInternalServerError)
		}
	}
}
```

#### Chi Router
```go
r := chi.NewRouter()
r.Use(httpguard.Middleware(guard, httpguard.Options{
	OverloadStatusCode: http.StatusServiceUnavailable,
	RetryAfterSeconds:  3,
}))
```

### Runtime Observability & Metrics

Inspect real-time concurrency metrics at any point:

```go
stats := guard.Stats()
log.Printf("In-Flight: %d | Queued: %d | Accepted: %d | Rejected: %d | Canceled: %d\n",
	stats.CurrentInFlight,
	stats.CurrentQueueSize,
	stats.AcceptedRequests,
	stats.RejectedRequests,
	stats.CanceledRequests,
)
```

---

## 2. Node.js & Express Setup

### Node.js Installation

Install using npm:

```bash
# If using the published package:
npm install @goloadguard/node

# Or install directly from the GitHub repository:
npm install github:Zihad-Bepari/GoLoadGuard
```

### 1-Line Plug-and-Play Middleware

To guard your entire Express server using ultra-capacity defaults (50,000 concurrency, 100,000 queue depth):

```javascript
const express = require('express');
const { loadGuardMiddleware } = require('@goloadguard/node');

const app = express();

// Protect all incoming routes
app.use(loadGuardMiddleware());

app.get('/api/orders', (req, res) => {
  res.json({ message: 'Processed under concurrency protection' });
});

app.listen(3000, () => console.log('Server running on port 3000'));
```

### Custom Instance & Route-Specific Guarding

To customize parameters or protect specific endpoints (e.g., heavy reporting APIs):

```typescript
import express from 'express';
import { LoadGuard, expressMiddleware } from '@goloadguard/node';

const app = express();

// Strict guard: 25 concurrent requests, 50 queue depth, 3-second queue timeout
const heavyApiGuard = new LoadGuard({
  maxConcurrency: 25,
  maxQueueSize: 50,
  queueTimeoutMs: 3000,
});

// Apply to heavy routes
app.get(
  '/api/generate-pdf',
  expressMiddleware(heavyApiGuard, {
    overloadStatus: 503,
    retryAfterSeconds: 5,
  }),
  async (req, res) => {
    // Heavy CPU/memory operation
    res.send({ status: 'Report generated successfully' });
  }
);
```

### Zero-Leak Socket Protection Verification

GoLoadGuard automatically binds to socket events (`finish`, `close`, `error`). If a client terminates their browser connection while waiting in queue or during execution, GoLoadGuard immediately releases the permit to ensure zero permit leakage.

---

## 3. NestJS Setup

### Global & Controller Guard Registration

GoLoadGuard provides a native NestJS Guard (`NestLoadGuard`):

```typescript
// app.module.ts
import { Module } from '@nestjs/common';
import { APP_GUARD } from '@nestjs/core';
import { NestLoadGuard } from '@goloadguard/node';
import { AppController } from './app.controller';

@Module({
  controllers: [AppController],
  providers: [
    {
      provide: APP_GUARD,
      useValue: new NestLoadGuard({
        maxConcurrency: 50,
        maxQueueSize: 100,
        queueTimeoutMs: 5000,
      }),
    },
  ],
})
export class AppModule {}
```

---

## 4. PHP (Native / Pure) Setup

### PHP Installation

In your `composer.json`, add GoLoadGuard as a VCS repository:

```json
{
  "repositories": [
    {
      "type": "vcs",
      "url": "https://github.com/Zihad-Bepari/GoLoadGuard.git"
    }
  ],
  "require": {
    "goloadguard/php": "*"
  }
}
```

Run composer update:

```bash
composer update
```

### Core Closure Usage (`run`)

```php
<?php

require_once __DIR__ . '/vendor/autoload.php';

use GoLoadGuard\Guard;
use GoLoadGuard\OverloadedException;
use GoLoadGuard\QueueTimeoutException;

// 15 max concurrency, 30 queue depth, 2000ms queue timeout
$guard = new Guard(
    maxConcurrency: 15,
    maxQueueSize: 30,
    queueTimeoutMs: 2000
);

try {
    $result = $guard->run(function () {
        // Critical section / DB query
        usleep(50000); // 50ms work
        return "Operation Successful";
    });

    echo $result;
} catch (OverloadedException $e) {
    http_response_code(503);
    header('Retry-After: 5');
    echo "Service Unavailable - Server Overloaded";
} catch (QueueTimeoutException $e) {
    http_response_code(504);
    echo "Gateway Timeout - Queue Wait Exceeded";
}
```

### Manual Acquire & Release Pattern

```php
$handle = $guard->acquire();
try {
    // Run work
} finally {
    $guard->release($handle);
}
```

---

## 5. Laravel Framework Setup

### Service Provider & Middleware Registration

#### Laravel 11 (`bootstrap/app.php`)
In Laravel 11, register the middleware alias inside `bootstrap/app.php`:

```php
use Illuminate\Foundation\Application;
use Illuminate\Foundation\Configuration\Middleware;
use GoLoadGuard\Laravel\LoadGuardMiddleware;

return Application::configure(basePath: dirname(__DIR__))
    ->withMiddleware(function (Middleware $middleware) {
        $middleware->alias([
            'loadguard' => LoadGuardMiddleware::class,
        ]);
    })
    ->create();
```

#### Laravel 9 & 10 (`app/Http/Kernel.php`)
In Laravel 10 or earlier, add the alias to `Kernel.php`:

```php
protected $middlewareAliases = [
    // ...
    'loadguard' => \GoLoadGuard\Laravel\LoadGuardMiddleware::class,
];
```

### Route Protection in Laravel

Protect individual routes or route groups:

```php
use Illuminate\Support\Facades\Route;

// Arguments: loadguard:maxConcurrency,maxQueueSize,queueTimeoutMs,retryAfterSeconds
Route::middleware(['loadguard:20,50,3000,5'])->group(function () {
    Route::post('/api/checkout', [CheckoutController::class, 'process']);
    Route::get('/api/reports/export', [ReportController::class, 'export']);
});
```

---

## Configuration Reference

| Option | Go Name | Node.js Name | PHP Argument | Default | Description |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Max Concurrency** | `MaxConcurrency` | `maxConcurrency` | `$maxConcurrency` | **Required** (`> 0`) | Maximum simultaneous in-flight operations allowed. |
| **Max Queue Size** | `MaxQueueSize` | `maxQueueSize` | `$maxQueueSize` | `0` | Maximum callers waiting in FIFO queue. `0` = immediate reject. |
| **Queue Timeout** | `context.WithTimeout` | `queueTimeoutMs` | `$queueTimeoutMs` | `0` *(no timeout)* | Maximum milliseconds a caller is permitted to wait in queue. |
| **Overload Status** | `OverloadStatusCode`| `overloadStatus` | N/A (HTTP 503) | `503` | HTTP status code returned when capacity is saturated. |
| **Retry After** | `RetryAfterSeconds` | `retryAfterSeconds`| `$retryAfterSeconds` | `0` *(disabled)* | Value of `Retry-After` HTTP header in seconds. |
| **Lock Path (PHP)** | N/A | N/A | `$lockPath` | `sys_get_temp_dir()`| Directory where atomic `flock` files are maintained. |
| **Stale Lease (PHP)**| N/A | N/A | `$staleTimeoutSeconds`| `60` | Auto-reclaim unreleased slots after hard worker crashes. |

---

## Production Deployment & Tuning

### cPanel & CloudLinux Shared Hosting

Shared hosting environments enforce strict CloudLinux Entry Process (EP) caps (typically 20–30 simultaneous PHP processes):
1. **Match Entry Process Limits**: Configure `maxConcurrency` to **15–20** and `maxQueueSize` to **30–50**.
2. **Prevent 508 Errors**: When incoming traffic surges, GoLoadGuard rejects excess callers at the gate with `503 Service Unavailable`, preventing CloudLinux from triggering `508 Resource Limit Reached` account freezes.
3. **Writable Directory**: Ensure `/tmp` or `storage/framework/` has write permissions for lock files.

### Docker & Containerized Microservices

When deploying containerized microservices in Kubernetes or Docker Swarm:
1. **Align with CPU Limits**: For CPU-bound workloads, set `MaxConcurrency = Container CPU Cores * 2`.
2. **Align with DB Connection Pools**: Set `MaxConcurrency` equal to or slightly lower than the maximum connection pool size (e.g., `db.SetMaxOpenConns(25)` -> `MaxConcurrency: 25`).

### Linux VPS & Nginx Reverse Proxy

When running behind Nginx:
1. GoLoadGuard returns `503 Service Unavailable` with `Retry-After: 5`.
2. Configure Nginx to forward the `Retry-After` header:
   ```nginx
   proxy_pass_header Retry-After;
   ```
3. Nginx and upstream load balancers will gracefully distribute traffic to healthy nodes.
