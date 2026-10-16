class OverloadedError extends Error {
  constructor(message = 'LoadGuard: system overloaded, capacity and queue full') {
    super(message);
    this.name = 'OverloadedError';
  }
}

class QueueTimeoutError extends Error {
  constructor(message = 'LoadGuard: queue wait timeout exceeded') {
    super(message);
    this.name = 'QueueTimeoutError';
  }
}

class ClosedError extends Error {
  constructor(message = 'LoadGuard: guard is closed') {
    super(message);
    this.name = 'ClosedError';
  }
}

class LoadGuard {
  constructor(config) {
    if (!config || config.maxConcurrency <= 0) {
      throw new Error('LoadGuard: maxConcurrency must be greater than 0');
    }
    this.maxConcurrency = Math.floor(config.maxConcurrency);
    this.maxQueueSize = Math.max(0, Math.floor(config.maxQueueSize ?? 0));
    this.queueTimeoutMs = Math.max(0, Math.floor(config.queueTimeoutMs ?? 0));

    this.inFlight = 0;
    this.queue = [];
    this.isClosed = false;

    this.stats = {
      currentInFlight: 0,
      currentQueueSize: 0,
      acceptedRequests: 0,
      rejectedRequests: 0,
      timedOutRequests: 0,
      canceledRequests: 0,
      closedRejections: 0,
    };
  }

  async acquire(signal) {
    if (this.isClosed) {
      this.stats.closedRejections++;
      throw new ClosedError();
    }

    if (signal?.aborted) {
      this.stats.canceledRequests++;
      throw new Error(signal.reason ?? 'Aborted');
    }

    if (this.inFlight < this.maxConcurrency && this.queue.length === 0) {
      this.inFlight++;
      this.stats.acceptedRequests++;
      return this.createRelease();
    }

    if (this.queue.length >= this.maxQueueSize) {
      this.stats.rejectedRequests++;
      throw new OverloadedError();
    }

    return new Promise((resolve, reject) => {
      let waiter;

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

  async run(fn, signal) {
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

  createRelease() {
    let released = false;
    return () => {
      if (released) return;
      released = true;

      while (this.queue.length > 0) {
        const next = this.queue.shift();
        if (next) {
          this.stats.acceptedRequests++;
          next.resolve(this.createRelease());
          return;
        }
      }

      this.inFlight = Math.max(0, this.inFlight - 1);
    };
  }

  close() {
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

  getStats() {
    return {
      ...this.stats,
      currentInFlight: this.inFlight,
      currentQueueSize: this.queue.length,
    };
  }
}

function expressMiddleware(guard, options = {}) {
  const status = options.overloadStatus ?? 503;

  return async (req, res, next) => {
    try {
      const release = await guard.acquire();
      let released = false;
      const safeRelease = () => {
        if (!released) {
          released = true;
          release();
        }
      };

      res.on('finish', safeRelease);
      res.on('close', safeRelease);
      next();
    } catch (err) {
      if (err instanceof OverloadedError || err instanceof ClosedError) {
        if (options.retryAfterSeconds) {
          res.setHeader('Retry-After', String(options.retryAfterSeconds));
        }
        res.status(status).send('Service Unavailable - System Overloaded');
        return;
      }
      next(err);
    }
  };
}

class NestLoadGuard {
  constructor(guard, overloadStatus = 503) {
    this.guard = guard;
    this.overloadStatus = overloadStatus;
  }

  async canActivate(context) {
    const http = context.switchToHttp();
    const res = http.getResponse();

    try {
      const release = await this.guard.acquire();
      let released = false;
      const safeRelease = () => {
        if (!released) {
          released = true;
          release();
        }
      };

      res.on('finish', safeRelease);
      res.on('close', safeRelease);
      return true;
    } catch (err) {
      if (err instanceof OverloadedError || err instanceof ClosedError) {
        if (res && typeof res.status === 'function') {
          res.status(this.overloadStatus).send('Service Unavailable - System Overloaded');
          return false;
        }
      }
      throw err;
    }
  }
}

module.exports = {
  LoadGuard,
  OverloadedError,
  QueueTimeoutError,
  ClosedError,
  expressMiddleware,
  NestLoadGuard,
};
