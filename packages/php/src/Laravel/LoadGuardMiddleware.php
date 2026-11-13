<?php

namespace GoLoadGuard\Laravel;

use Closure;
use GoLoadGuard\Guard;
use GoLoadGuard\OverloadedException;
use GoLoadGuard\QueueTimeoutException;
use Illuminate\Http\Request;
use Symfony\Component\HttpFoundation\Response;

class LoadGuardMiddleware
{
    /**
     * Handle an incoming request with concurrency protection.
     *
     * @param Request $request
     * @param Closure $next
     * @param int $maxConcurrency Maximum concurrent requests allowed
     * @param int $maxQueueSize Maximum queue capacity (default: 0)
     * @param int $queueTimeoutMs Max queue wait in ms (default: 0)
     * @param int $retryAfterSeconds Retry-After header value in seconds (default: 5)
     * @return mixed
     */
    public function handle(
        Request $request,
        Closure $next,
        int $maxConcurrency = 20,
        int $maxQueueSize = 50,
        int $queueTimeoutMs = 3000,
        int $retryAfterSeconds = 5,
        string $guardName = 'default'
    ) {
        $storagePath = function_exists('storage_path') ? storage_path('framework') : sys_get_temp_dir();
        $guard = new Guard(
            (int)$maxConcurrency,
            (int)$maxQueueSize,
            (int)$queueTimeoutMs,
            $storagePath,
            60,
            $guardName
        );

        try {
            $handle = $guard->acquire();
        } catch (OverloadedException | QueueTimeoutException $e) {
            return response('Service Unavailable - Server Overloaded', Response::HTTP_SERVICE_UNAVAILABLE, [
                'Retry-After' => (string)$retryAfterSeconds,
            ]);
        }

        try {
            return $next($request);
        } finally {
            $guard->release($handle);
        }
    }
}
