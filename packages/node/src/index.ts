// Configuration options for LoadGuard
export interface LoadGuardConfig {
  maxConcurrency: number;
  maxQueueSize?: number;
  queueTimeoutMs?: number;
}

// Runtime metrics snapshot
export interface LoadGuardStats {
  currentInFlight: number;
  currentQueueSize: number;
  acceptedRequests: number;
  rejectedRequests: number;
  timedOutRequests: number;
  canceledRequests: number;
  closedRejections: number;
}

// Error thrown when capacity and queue are full
export class OverloadedError extends Error {
  constructor(message = 'LoadGuard: system overloaded, capacity and queue full') {
    super(message);
    this.name = 'OverloadedError';
  }
}

// Error thrown when queue timeout is exceeded
export class QueueTimeoutError extends Error {
  constructor(message = 'LoadGuard: queue wait timeout exceeded') {
    super(message);
    this.name = 'QueueTimeoutError';
  }
}

// Error thrown when guard is closed
export class ClosedError extends Error {
  constructor(message = 'LoadGuard: guard is closed') {
    super(message);
    this.name = 'ClosedError';
  }
}

interface Waiter {
  resolve: (release: () => void) => void;
  reject: (err: Error) => void;
  timer?: NodeJS.Timeout;
}

// In-process concurrency limiter with FIFO queue
export class LoadGuard {
  private readonly maxConcurrency: number;
  private readonly maxQueueSize: number;
  private readonly queueTimeoutMs: number;

  private inFlight = 0;
  private queue: Waiter[] = [];
  private isClosed = false;

  private stats: LoadGuardStats = {
    currentInFlight: 0,
    currentQueueSize: 0,
    acceptedRequests: 0,
    rejectedRequests: 0,
    timedOutRequests: 0,
    canceledRequests: 0,
    closedRejections: 0,
  };

  constructor(config: LoadGuardConfig) {
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
  public async acquire(signal?: AbortSignal): Promise<() => void> {
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
    return new Promise<() => void>((resolve, reject) => {
      let waiter: Waiter;

      const cleanup = () => {
        if (waiter.timer) clearTimeout(waiter.timer);
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
          if (signal) signal.removeEventListener('abort', onAbort);
          if (waiter.timer) clearTimeout(waiter.timer);
          resolve(rel);
        },
        reject: (err) => {
          if (signal) signal.removeEventListener('abort', onAbort);
          if (waiter.timer) clearTimeout(waiter.timer);
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
  public async run<T>(fn: () => Promise<T> | T, signal?: AbortSignal): Promise<T> {
    const release = await this.acquire(signal);
    try {
      if (signal?.aborted) {
        throw new Error(signal.reason ?? 'Aborted');
      }
      return await fn();
    } finally {
      release();
    }
  }

  private createRelease(): () => void {
    let released = false;
    return () => {
      if (released) return;
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
  public close(): void {
    if (this.isClosed) return;
    this.isClosed = true;

    while (this.queue.length > 0) {
      const waiter = this.queue.shift();
      if (waiter) {
        if (waiter.timer) clearTimeout(waiter.timer);
        this.stats.closedRejections++;
        waiter.reject(new ClosedError());
      }
    }
  }

  /**
   * Returns a snapshot of runtime metrics.
   */
  public getStats(): LoadGuardStats {
    return {
      ...this.stats,
      currentInFlight: this.inFlight,
      currentQueueSize: this.queue.length,
    };
  }
}

let _defaultGuard: LoadGuard | undefined;

/**
 * Returns the globally registered default LoadGuard instance, if created via loadGuardMiddleware.
 */
export function getDefaultGuard(): LoadGuard | undefined {
  return _defaultGuard;
}

// Express middleware for concurrency limiting
export function expressMiddleware(
  guard: LoadGuard,
  options: { overloadStatus?: number; retryAfterSeconds?: number; bypassPaths?: string[] } = {}
) {
  const status = options.overloadStatus ?? 503;

  return async (req: any, res: any, next: (err?: any) => void) => {
    // ignore already closed connections
    if (req.destroyed || res.destroyed || (res.socket && res.socket.destroyed)) {
      return;
    }

    // bypass path check
    if (options.bypassPaths && options.bypassPaths.some((p) => req.path?.includes(p))) {
      return next();
    }

    let release: (() => void) | undefined;
    try {
      release = await guard.acquire();
    } catch (err: any) {
      if (
        err instanceof OverloadedError ||
        err instanceof ClosedError ||
        err?.name === 'OverloadedError' ||
        err?.name === 'ClosedError'
      ) {
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
        release!();
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
export function loadGuardMiddleware(
  options: Partial<LoadGuardConfig> & {
    overloadStatus?: number;
    retryAfterSeconds?: number;
    bypassPaths?: string[];
  } = {}
) {
  const guard = new LoadGuard({
    maxConcurrency: options.maxConcurrency ?? 50000,
    maxQueueSize: options.maxQueueSize ?? 100000,
    queueTimeoutMs: options.queueTimeoutMs ?? 15000,
  });
  _defaultGuard = guard;
  return expressMiddleware(guard, options);
}

// Lazy loader for NestJS Guard
let _nestGuard: typeof import('./nest.guard').NestLoadGuard | undefined;
export const NestLoadGuard: typeof import('./nest.guard').NestLoadGuard = new Proxy(
  function () {} as any,
  {
    get(_target, prop) {
      if (!_nestGuard) {
        _nestGuard = require('./nest.guard').NestLoadGuard;
      }
      return (_nestGuard as any)[prop];
    },
    construct(_target, args) {
      if (!_nestGuard) {
        _nestGuard = require('./nest.guard').NestLoadGuard;
      }
      return new (_nestGuard as any)(...args);
    },
  }
);
