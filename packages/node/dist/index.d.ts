export interface LoadGuardConfig {
    maxConcurrency: number;
    maxQueueSize?: number;
    queueTimeoutMs?: number;
}
export interface LoadGuardStats {
    currentInFlight: number;
    currentQueueSize: number;
    acceptedRequests: number;
    rejectedRequests: number;
    timedOutRequests: number;
    canceledRequests: number;
    closedRejections: number;
}
export declare class OverloadedError extends Error {
    constructor(message?: string);
}
export declare class QueueTimeoutError extends Error {
    constructor(message?: string);
}
export declare class ClosedError extends Error {
    constructor(message?: string);
}
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
export declare function expressMiddleware(guard: LoadGuard, options?: {
    overloadStatus?: number;
    retryAfterSeconds?: number;
    bypassPaths?: string[];
}): (req: any, res: any, next: (err?: any) => void) => Promise<void>;
export declare function loadGuardMiddleware(options?: Partial<LoadGuardConfig> & {
    overloadStatus?: number;
    retryAfterSeconds?: number;
    bypassPaths?: string[];
}): (req: any, res: any, next: (err?: any) => void) => Promise<void>;
export declare const NestLoadGuard: typeof import('./nest.guard').NestLoadGuard;
//# sourceMappingURL=index.d.ts.map