<?php

namespace GoLoadGuard;

use Exception;

class OverloadedException extends Exception {}
class QueueTimeoutException extends Exception {}
class ClosedException extends Exception {}

// Concurrency limiter for PHP using file locks
class Guard
{
    private int $maxConcurrency;
    private int $maxQueueSize;
    private int $queueTimeoutMs;
    private string $lockFile;
    private int $staleTimeoutSeconds;

    public function __construct(
        int $maxConcurrency,
        int $maxQueueSize = 0,
        int $queueTimeoutMs = 0,
        ?string $lockPath = null,
        int $staleTimeoutSeconds = 60,
        ?string $guardName = null
    ) {
        if ($maxConcurrency <= 0) {
            throw new \InvalidArgumentException("GoLoadGuard: maxConcurrency must be greater than 0");
        }

        $this->maxConcurrency = $maxConcurrency;
        $this->maxQueueSize = max(0, $maxQueueSize);
        // Default to a safe 3000ms max timeout if 0 is passed, preventing infinite PHP-FPM worker lockup
        $this->queueTimeoutMs = $queueTimeoutMs > 0 ? $queueTimeoutMs : 3000;
        $this->staleTimeoutSeconds = max(5, $staleTimeoutSeconds);

        // Auto-detect private project storage (Laravel storage_path) over shared /tmp for cPanel multi-tenant isolation
        if ($lockPath !== null) {
            $dir = $lockPath;
        } elseif (function_exists('storage_path') && is_dir(storage_path())) {
            $dir = storage_path('framework/cache');
            if (!is_dir($dir)) @mkdir($dir, 0755, true);
        } else {
            $dir = sys_get_temp_dir();
        }
        $scopeKey = $guardName ?? 'default';
        $appScope = md5(__DIR__ . '_' . $scopeKey);
        $this->lockFile = rtrim($dir, DIRECTORY_SEPARATOR) . DIRECTORY_SEPARATOR . 'goloadguard_' . $appScope . '.lock';
    }

    // Execute callback within guard
    public function run(callable $callback)
    {
        $handle = $this->acquire();
        try {
            return $callback();
        } finally {
            $this->release($handle);
        }
    }

    // Acquire slot or wait in queue
    public function acquire()
    {
        $fp = fopen($this->lockFile, 'c+');
        if (!$fp) {
            throw new \RuntimeException("GoLoadGuard: Failed to open lock file: {$this->lockFile}");
        }

        $startTime = microtime(true);
        $timeoutMs = $this->queueTimeoutMs > 0 ? $this->queueTimeoutMs : 3000;

        while (true) {
            // Non-blocking exclusive lock check
            if (flock($fp, LOCK_EX | LOCK_NB)) {
                $content = stream_get_contents($fp, -1, 0);
                $state = $this->parseState($content);

                if ($state['active'] < $this->maxConcurrency) {
                    $slotId = uniqid('', true);
                    $state['slots'][] = [
                        'id' => $slotId,
                        'pid' => function_exists('getmypid') ? getmypid() : 0,
                        'ts' => microtime(true),
                    ];
                    $state['active'] = count($state['slots']);
                    $this->writeState($fp, $state);
                    flock($fp, LOCK_UN);
                    return ['fp' => $fp, 'id' => $slotId];
                }

                // If queue is disabled or capacity is full, reject immediately
                if ($this->maxQueueSize <= 0 || $state['queued'] >= $this->maxQueueSize) {
                    flock($fp, LOCK_UN);
                    fclose($fp);
                    throw new OverloadedException("GoLoadGuard: system overloaded, capacity and queue full");
                }

                // Increment queued counter
                $state['queued']++;
                $this->writeState($fp, $state);
                flock($fp, LOCK_UN);
                break; // Now waiting
            }

            if ((microtime(true) - $startTime) * 1000 >= $timeoutMs) {
                fclose($fp);
                throw new OverloadedException("GoLoadGuard: lock contention timeout, system overloaded");
            }

            usleep(2000); // 2ms backoff
        }

        // Wait in queue loop
        while (true) {
            $elapsedMs = (microtime(true) - $startTime) * 1000;
            if ($elapsedMs >= $timeoutMs) {
                $this->decrementQueued($fp);
                fclose($fp);
                throw new QueueTimeoutException("GoLoadGuard: queue wait timeout exceeded");
            }

            usleep(5000); // 5ms polling interval

            if (flock($fp, LOCK_EX | LOCK_NB)) {
                $content = stream_get_contents($fp, -1, 0);
                $state = $this->parseState($content);

                if ($state['active'] < $this->maxConcurrency) {
                    $slotId = uniqid('', true);
                    $state['slots'][] = [
                        'id' => $slotId,
                        'pid' => function_exists('getmypid') ? getmypid() : 0,
                        'ts' => microtime(true),
                    ];
                    $state['active'] = count($state['slots']);
                    $state['queued'] = max(0, $state['queued'] - 1);
                    $this->writeState($fp, $state);
                    flock($fp, LOCK_UN);
                    return ['fp' => $fp, 'id' => $slotId];
                }

                flock($fp, LOCK_UN);
            }
        }
    }

    // Release acquired slot
    public function release($handle): void
    {
        $slotId = null;
        if (is_array($handle)) {
            $fp = $handle['fp'] ?? null;
            $slotId = $handle['id'] ?? null;
        } else {
            $fp = $handle;
        }

        if (!is_resource($fp)) {
            return;
        }

        $retries = 0;
        while ($retries < 50) {
            if (flock($fp, LOCK_EX)) {
                $content = stream_get_contents($fp, -1, 0);
                $state = $this->parseState($content);

                if (!empty($state['slots'])) {
                    $found = false;
                    if ($slotId !== null) {
                        foreach ($state['slots'] as $k => $slot) {
                            if (isset($slot['id']) && $slot['id'] === $slotId) {
                                unset($state['slots'][$k]);
                                $state['slots'] = array_values($state['slots']);
                                $found = true;
                                break;
                            }
                        }
                    }
                    if (!$found) {
                        array_shift($state['slots']);
                    }
                    $state['active'] = count($state['slots']);
                } else {
                    $state['active'] = max(0, $state['active'] - 1);
                }
                $this->writeState($fp, $state);

                flock($fp, LOCK_UN);
                fclose($fp);
                break;
            }
            $retries++;
            usleep(1000);
        }
        if ($retries >= 50 && is_resource($fp)) {
            @fclose($fp);
        }
    }

    private function decrementQueued($fp): void
    {
        if (flock($fp, LOCK_EX)) {
            $content = stream_get_contents($fp, -1, 0);
            $state = $this->parseState($content);
            $state['queued'] = max(0, $state['queued'] - 1);
            $this->writeState($fp, $state);
            flock($fp, LOCK_UN);
        }
    }

    private function parseState(string $raw): array
    {
        $data = json_decode($raw, true);
        if (!is_array($data)) {
            return ['active' => 0, 'queued' => 0, 'slots' => []];
        }

        $now = microtime(true);
        $slots = [];
        if (isset($data['slots']) && is_array($data['slots'])) {
            foreach ($data['slots'] as $slot) {
                if (!is_array($slot) || empty($slot['ts'])) {
                    continue;
                }
                // Check if lease exceeded stale timeout
                if ($now - (float)$slot['ts'] > $this->staleTimeoutSeconds) {
                    continue;
                }
                // Check if process still alive if posix_kill exists
                if (function_exists('posix_kill') && !empty($slot['pid'])) {
                    if (!@posix_kill((int)$slot['pid'], 0)) {
                        continue;
                    }
                }
                $slots[] = $slot;
            }
            $active = count($slots);
        } else {
            $active = max(0, (int)($data['active'] ?? 0));
        }

        return [
            'active' => $active,
            'queued' => max(0, (int)($data['queued'] ?? 0)),
            'slots' => $slots,
        ];
    }

    private function writeState($fp, array $state): void
    {
        ftruncate($fp, 0);
        rewind($fp);
        fwrite($fp, json_encode($state));
        fflush($fp);
    }
}
