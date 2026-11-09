/**
 * Configuration options for LoadGuard.
 */
export interface LoadGuardConfig {
    /**
     * Maximum number of concurrent operations allowed.
     * Must be greater than 0.
     */
    maxConcurrency: number;
    /**
     * Maximum queue depth for waiting callers.
     * A value of 0 means immediate rejection when slots are saturated.
     */
    maxQueueSize?: number;
    /**
     * Maximum duration (in milliseconds) a caller is permitted to wait in queue.
     * Defaults to 0 (no timeout, waits until granted or aborted).
     */
    queueTimeoutMs?: number;
}
/**
 * Point-in-time metrics snapshot.
 */
export interface LoadGuardStats {
    currentInFlight: number;
    currentQueueSize: number;
    acceptedRequests: number;
    rejectedRequests: number;
    timedOutRequests: number;
    canceledRequests: number;
    closedRejections: number;
}
/**
 * Custom error thrown when both concurrency capacity and queue are exhausted.
 */
export declare class OverloadedError extends Error {
    constructor(message?: string);
}
/**
 * Custom error thrown when waiting in queue exceeds queueTimeoutMs.
 */
export declare class QueueTimeoutError extends Error {
    constructor(message?: string);
}
/**
 * Custom error thrown when acquiring from a closed Guard.
 */
export declare class ClosedError extends Error {
    constructor(message?: string);
}
/**
 * In-process Concurrency Guard with strictly bounded FIFO queue.
 */
export declare class LoadGuard {
    private readonly maxConcurrency;
    private readonly maxQueueSize;
    private readonly queueTimeoutMs;
    private inFlight;
    private queue;
    private isClosed;
    private stats;
    constructor(config: LoadGuardConfig);
    /**
     * Acquire a slot or wait in FIFO queue. Returns a release function.
     */
    acquire(signal?: AbortSignal): Promise<() => void>;
    /**
     * Execute callback function within the concurrency guard.
     */
    run<T>(fn: () => Promise<T> | T, signal?: AbortSignal): Promise<T>;
    private createRelease;
    /**
     * Close the guard, preventing new admissions and unblocking waiters with ClosedError.
     */
    close(): void;
    /**
     * Returns a snapshot of runtime metrics.
     */
    getStats(): LoadGuardStats;
}
/**
 * Returns the globally registered default LoadGuard instance, if created via loadGuardMiddleware.
 */
export declare function getDefaultGuard(): LoadGuard | undefined;
/**
 * Express / Connect compatible HTTP Middleware.
 * Hardened with Zero-Leak Socket lifecycle protection against client disconnections.
 */
export declare function expressMiddleware(guard: LoadGuard, options?: {
    overloadStatus?: number;
    retryAfterSeconds?: number;
    bypassPaths?: string[];
}): (req: any, res: any, next: (err?: any) => void) => Promise<void>;
/**
 * Plug-and-play Express Middleware with intelligent ultra-capacity defaults.
 * Allows zero-config integration across any Node.js / Express application.
 */
export declare function loadGuardMiddleware(options?: Partial<LoadGuardConfig> & {
    overloadStatus?: number;
    retryAfterSeconds?: number;
    bypassPaths?: string[];
}): (req: any, res: any, next: (err?: any) => void) => Promise<void>;
export declare const NestLoadGuard: typeof import('./nest.guard').NestLoadGuard;
//# sourceMappingURL=index.d.ts.map