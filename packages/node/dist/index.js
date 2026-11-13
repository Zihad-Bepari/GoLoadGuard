"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.NestLoadGuard = exports.LoadGuard = exports.ClosedError = exports.QueueTimeoutError = exports.OverloadedError = void 0;
exports.getDefaultGuard = getDefaultGuard;
exports.expressMiddleware = expressMiddleware;
exports.loadGuardMiddleware = loadGuardMiddleware;
// Error thrown when capacity and queue are full
class OverloadedError extends Error {
    constructor(message = 'LoadGuard: system overloaded, capacity and queue full') {
        super(message);
        this.name = 'OverloadedError';
    }
}
exports.OverloadedError = OverloadedError;
// Error thrown when queue timeout is exceeded
class QueueTimeoutError extends Error {
    constructor(message = 'LoadGuard: queue wait timeout exceeded') {
        super(message);
        this.name = 'QueueTimeoutError';
    }
}
exports.QueueTimeoutError = QueueTimeoutError;
// Error thrown when guard is closed
class ClosedError extends Error {
    constructor(message = 'LoadGuard: guard is closed') {
        super(message);
        this.name = 'ClosedError';
    }
}
exports.ClosedError = ClosedError;
// In-process concurrency limiter with FIFO queue
class LoadGuard {
    maxConcurrency;
    maxQueueSize;
    queueTimeoutMs;
    inFlight = 0;
    queue = [];
    isClosed = false;
    stats = {
        currentInFlight: 0,
        currentQueueSize: 0,
        acceptedRequests: 0,
        rejectedRequests: 0,
        timedOutRequests: 0,
        canceledRequests: 0,
        closedRejections: 0,
    };
    constructor(config) {
        if (!config || config.maxConcurrency <= 0) {
            throw new Error('LoadGuard: maxConcurrency must be greater than 0');
        }
        this.maxConcurrency = Math.floor(config.maxConcurrency);
        this.maxQueueSize = Math.max(0, Math.floor(config.maxQueueSize ?? 0));
        this.queueTimeoutMs = Math.max(0, Math.floor(config.queueTimeoutMs ?? 0));
    }
    /**
     * Acquire a slot or wait in FIFO queue. Returns a release function.
     */
    async acquire(signal) {
        if (this.isClosed) {
            this.stats.closedRejections++;
            throw new ClosedError();
        }
        if (signal?.aborted) {
            this.stats.canceledRequests++;
            throw new Error(signal.reason ?? 'Aborted');
        }
        // Fast path: immediate admission
        if (this.inFlight < this.maxConcurrency && this.queue.length === 0) {
            this.inFlight++;
            this.stats.acceptedRequests++;
            return this.createRelease();
        }
        // Overload check
        if (this.queue.length >= this.maxQueueSize) {
            this.stats.rejectedRequests++;
            throw new OverloadedError();
        }
        // Enqueue waiter
        return new Promise((resolve, reject) => {
            let waiter;
            const cleanup = () => {
                if (waiter.timer)
                    clearTimeout(waiter.timer);
                const idx = this.queue.indexOf(waiter);
                if (idx !== -1) {
                    this.queue.splice(idx, 1);
                }
            };
            const onAbort = () => {
                cleanup();
                this.stats.canceledRequests++;
                reject(new Error(signal?.reason ?? 'Aborted'));
            };
            waiter = {
                resolve: (rel) => {
                    if (signal)
                        signal.removeEventListener('abort', onAbort);
                    if (waiter.timer)
                        clearTimeout(waiter.timer);
                    resolve(rel);
                },
                reject: (err) => {
                    if (signal)
                        signal.removeEventListener('abort', onAbort);
                    if (waiter.timer)
                        clearTimeout(waiter.timer);
                    reject(err);
                },
            };
            if (this.queueTimeoutMs > 0) {
                waiter.timer = setTimeout(() => {
                    cleanup();
                    this.stats.timedOutRequests++;
                    waiter.reject(new QueueTimeoutError());
                }, this.queueTimeoutMs);
            }
            if (signal) {
                signal.addEventListener('abort', onAbort, { once: true });
            }
            this.queue.push(waiter);
        });
    }
    /**
     * Execute callback function within the concurrency guard.
     */
    async run(fn, signal) {
        const release = await this.acquire(signal);
        try {
            if (signal?.aborted) {
                throw new Error(signal.reason ?? 'Aborted');
            }
            return await fn();
        }
        finally {
            release();
        }
    }
    createRelease() {
        let released = false;
        return () => {
            if (released)
                return;
            released = true;
            // Handoff to next queued waiter
            while (this.queue.length > 0) {
                const next = this.queue.shift();
                if (next) {
                    this.stats.acceptedRequests++;
                    next.resolve(this.createRelease());
                    return; // slot handed off directly
                }
            }
            this.inFlight = Math.max(0, this.inFlight - 1);
        };
    }
    /**
     * Close the guard, preventing new admissions and unblocking waiters with ClosedError.
     */
    close() {
        if (this.isClosed)
            return;
        this.isClosed = true;
        while (this.queue.length > 0) {
            const waiter = this.queue.shift();
            if (waiter) {
                if (waiter.timer)
                    clearTimeout(waiter.timer);
                this.stats.closedRejections++;
                waiter.reject(new ClosedError());
            }
        }
    }
    /**
     * Returns a snapshot of runtime metrics.
     */
    getStats() {
        return {
            ...this.stats,
            currentInFlight: this.inFlight,
            currentQueueSize: this.queue.length,
        };
    }
}
exports.LoadGuard = LoadGuard;
let _defaultGuard;
/**
 * Returns the globally registered default LoadGuard instance, if created via loadGuardMiddleware.
 */
function getDefaultGuard() {
    return _defaultGuard;
}
// Express middleware for concurrency limiting
function expressMiddleware(guard, options = {}) {
    const status = options.overloadStatus ?? 503;
    return async (req, res, next) => {
        // ignore already closed connections
        if (req.destroyed || res.destroyed || (res.socket && res.socket.destroyed)) {
            return;
        }
        // bypass path check
        if (options.bypassPaths && options.bypassPaths.some((p) => req.path?.includes(p))) {
            return next();
        }
        let release;
        try {
            release = await guard.acquire();
        }
        catch (err) {
            if (err instanceof OverloadedError ||
                err instanceof ClosedError ||
                err?.name === 'OverloadedError' ||
                err?.name === 'ClosedError') {
                if (options.retryAfterSeconds) {
                    res.setHeader('Retry-After', String(options.retryAfterSeconds));
                }
                res.status(status).send('Service Unavailable - System Overloaded');
                return;
            }
            return next(err);
        }
        // check if socket aborted while waiting in queue
        if (req.destroyed || res.destroyed || (res.socket && res.socket.destroyed)) {
            release();
            return;
        }
        let released = false;
        const safeRelease = () => {
            if (!released) {
                released = true;
                release();
            }
        };
        res.on('finish', safeRelease);
        res.on('close', safeRelease);
        req.on('close', safeRelease);
        req.on('error', safeRelease);
        res.on('error', safeRelease);
        next();
    };
}
// Factory helper for Express middleware
function loadGuardMiddleware(options = {}) {
    const guard = new LoadGuard({
        maxConcurrency: options.maxConcurrency ?? 50000,
        maxQueueSize: options.maxQueueSize ?? 100000,
        queueTimeoutMs: options.queueTimeoutMs ?? 15000,
    });
    _defaultGuard = guard;
    return expressMiddleware(guard, options);
}
// Lazy loader for NestJS Guard
let _nestGuard;
exports.NestLoadGuard = new Proxy(function () { }, {
    get(_target, prop) {
        if (!_nestGuard) {
            _nestGuard = require('./nest.guard').NestLoadGuard;
        }
        return _nestGuard[prop];
    },
    construct(_target, args) {
        if (!_nestGuard) {
            _nestGuard = require('./nest.guard').NestLoadGuard;
        }
        return new _nestGuard(...args);
    },
});
//# sourceMappingURL=index.js.map