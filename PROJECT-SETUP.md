# GoLoadGuard — Setup & Integration Guide

A simple, step-by-step guide to installing and using **GoLoadGuard** in **Go**, **Node.js**, **NestJS**, **PHP**, and **Laravel**.

---

## 📑 Table of Contents
1. [Installation Summary](#1-installation-summary)
2. [Go Setup](#2-go-setup)
3. [Node.js & Express Setup](#3-nodejs--express-setup)
4. [NestJS Setup](#4-nestjs-setup)
5. [PHP (Plain) Setup](#5-php-plain-setup)
6. [Laravel Setup](#6-laravel-setup)
7. [Configuration Options](#7-configuration-options)

---

## 1. Installation Summary

| Platform | Command | Notes |
| :--- | :--- | :--- |
| **Go** | `go get github.com/Zihad-Bepari/GoLoadGuard` | Go 1.21 or higher |
| **Node.js** | `npm install github:Zihad-Bepari/GoLoadGuard` | Node 18 or higher |
| **NestJS** | `npm install github:Zihad-Bepari/GoLoadGuard` | NestJS 9 or higher |
| **PHP / Laravel** | `composer require goloadguard/goloadguard:dev-main` | PHP 8.0 or higher |

---

## 2. Go Setup

### Install:
```bash
go get github.com/Zihad-Bepari/GoLoadGuard
```

### Wrapping Code Tasks:
```go
package main

import (
    "context"
    "fmt"
    "github.com/Zihad-Bepari/GoLoadGuard"
)

func main() {
    guard, err := goloadguard.New(goloadguard.Config{
        MaxConcurrency: 10, // Max 10 tasks at once
        MaxQueueSize:   20, // Max 20 waiting in queue
    })
    if err != nil {
        panic(err)
    }

    // Execute protected code
    err = guard.Do(context.Background(), func(ctx context.Context) error {
        fmt.Println("Doing heavy work safely...")
        return nil
    })
}
```

### HTTP Server Middleware:
```go
package main

import (
    "net/http"
    "github.com/Zihad-Bepari/GoLoadGuard"
    "github.com/Zihad-Bepari/GoLoadGuard/httpguard"
)

func main() {
    guard, _ := goloadguard.New(goloadguard.Config{
        MaxConcurrency: 50,
        MaxQueueSize:   100,
    })

    mux := http.NewServeMux()
    mux.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("Safe response"))
    })

    http.ListenAndServe(":8080", httpguard.Middleware(guard)(mux))
}
```

---

## 3. Node.js & Express Setup

### Install:
```bash
npm install github:Zihad-Bepari/GoLoadGuard
```

### Express Middleware (`app.js`):
```javascript
const express = require('express');
const { loadGuardMiddleware } = require('@goloadguard/node');

const app = express();

// Protect all routes
app.use(loadGuardMiddleware({
  maxConcurrency: 30, // Max 30 parallel requests
  maxQueueSize: 50,   // Max 50 in queue
  queueTimeoutMs: 3000 // 3-second queue wait timeout
}));

app.get('/api/users', (req, res) => {
  res.json({ message: 'Success' });
});

app.listen(3000);
```

---

## 4. NestJS Setup

### Route Guard:
```typescript
import { Controller, Get, UseGuards } from '@nestjs/common';
import { LoadGuard } from '@goloadguard/node';
import { NestLoadGuard } from '@goloadguard/node/dist/nest.guard';

const guard = new LoadGuard({ maxConcurrency: 15, maxQueueSize: 30 });

@Controller('reports')
export class ReportsController {
  @Get()
  @UseGuards(new NestLoadGuard(guard))
  getReports() {
    return { data: 'Generated safely' };
  }
}
```

---

## 5. PHP (Plain) Setup

### Install:
```bash
composer require goloadguard/goloadguard:dev-main
```

### Usage:
```php
require 'vendor/autoload.php';

use GoLoadGuard\Guard;

$guard = new Guard(
    maxConcurrency: 10,
    maxQueueSize: 20,
    queueTimeoutMs: 3000
);

$result = $guard->run(function () {
    // Heavy work here
    return "Done";
});

echo $result;
```

---

## 6. Laravel Setup

The ServiceProvider is automatically loaded in Laravel.

### Protect Routes in `routes/api.php`:
```php
use Illuminate\Support\Facades\Route;

// Max 20 concurrent requests, max 50 in queue
Route::middleware('loadguard:20,50')->group(function () {
    Route::get('/checkout', [CheckoutController::class, 'process']);
});
```

---

## 7. Configuration Options

| Option | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `maxConcurrency` | int | Required | Maximum allowed simultaneous active operations |
| `maxQueueSize` | int | `0` | Maximum requests allowed to wait in the queue |
| `queueTimeoutMs` | int | `3000` | Max milliseconds a request can wait in queue before rejection |
| `overloadStatus` | int | `503` | HTTP status code returned when overloaded |
| `retryAfterSeconds` | int | `5` | `Retry-After` HTTP header value in seconds |
