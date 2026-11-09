<?php

namespace GoLoadGuard;

use Exception;

class OverloadedException extends Exception {}
class QueueTimeoutException extends Exception {}
class ClosedException extends Exception {}

/**
 * Universal In-Process Concurrency Guard for PHP and Laravel.
 * Uses atomic kernel-level file locks (`flock`) in /tmp or storage,
 * making it 100% compliant with cPanel, Shared Hosting, and Cloud environments
 * without requiring root access, background daemons, or Redis.
 */
class Guard
{
    private int $maxConcurrency;
    private int $maxQueueSize;
    private int $queueTimeoutMs;
    private string $lockFile;
    private int $staleTimeoutSeconds;

    /**
     * @param int $maxConcurrency Maximum concurrent operations (> 0).
     * @param int $maxQueueSize Maximum waiting queue capacity (>= 0).
     * @param int $queueTimeoutMs Queue wait timeout in milliseconds (0 = no timeout).
     * @param string|null $lockPath Directory or full file path for lock tracking.
     * @param int $staleTimeoutSeconds Maximum lifetime of an unreleased slot before auto-reclaiming (prevents deadlocks on fatal process crashes).
     */
    public function __construct(
        int $maxConcurrency,
        int $maxQueueSize = 0,
        int $queueTimeoutMs = 0,
        ?string $lockPath = null,
        int $staleTimeoutSeconds = 60
    ) {
        if ($maxConcurrency <= 0) {
            throw new \InvalidArgumentException("GoLoadGuard: maxConcurrency must be greater than 0");
        }

        $this->maxConcurrency = $maxConcurrency;
        $this->maxQueueSize = max(0, $maxQueueSize);
        $this->queueTimeoutMs = max(0, $queueTimeoutMs);
        $this->staleTimeoutSeconds = max(5, $staleTimeoutSeconds);

        // Default to /tmp or sys_get_temp_dir (Universally writable on cPanel/Shared hosting)
        $dir = $lockPath ?? sys_get_temp_dir();
        $appScope = md5(__DIR__ . '_' . $this->maxConcurrency . '_' . $this->maxQueueSize);
        $this->lockFile = rtrim($dir, DIRECTORY_SEPARATOR) . DIRECTORY_SEPARATOR . 'goloadguard_' . $appScope . '.lock';
    }

    /**
     * Executes a callable within the concurrency boundary.
     * Guarantees release even on thrown exceptions.
     *
     * @template T
     * @param callable(): T $callback
     * @return T
     * @throws OverloadedException|QueueTimeoutException
     */
    public function run(callable $callback)
    {
        $handle = $this->acquire();
        try {
            return $callback();
        } finally {
            $this->release($handle);
        }
    }

    /**
     * Acquire a permit slot. Returns an open file handle on success.
     *
     * @return resource
     * @throws OverloadedException|QueueTimeoutException
     */
    public function acquire()
    {
        $fp = fopen($this->lockFile, 'c+');
        if (!$fp) {
            throw new \RuntimeException("GoLoadGuard: Failed to open lock file: {$this->lockFile}");
        }

        $startTime = microtime(true);

        while (true) {
            // Non-blocking exclusive lock check
            if (flock($fp, LOCK_EX | LOCK_NB)) {
                $content = stream_get_contents($fp, -1, 0);
                $state = $this->parseState($content);

                if ($state['active'] < $this->maxConcurrency) {
                    $state['slots'][] = [
                        'id' => uniqid('', true),
                        'pid' => function_exists('getmypid') ? getmypid() : 0,
                        'ts' => microtime(true),
                    ];
                    $state['active'] = count($state['slots']);
                    $this->writeState($fp, $state);
                    flock($fp, LOCK_UN);
                    return $fp;
                }

                // Check queue capacity
                if ($state['queued'] >= $this->maxQueueSize) {
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

            usleep(2000); // 2ms backoff
        }

        // Wait in queue loop
        while (true) {
            if ($this->queueTimeoutMs > 0) {
                $elapsedMs = (microtime(true) - $startTime) * 1000;
                if ($elapsedMs >= $this->queueTimeoutMs) {
                    $this->decrementQueued($fp);
                    fclose($fp);
                    throw new QueueTimeoutException("GoLoadGuard: queue wait timeout exceeded");
                }
            }

            usleep(5000); // 5ms polling interval

            if (flock($fp, LOCK_EX | LOCK_NB)) {
                $content = stream_get_contents($fp, -1, 0);
                $state = $this->parseState($content);

                if ($state['active'] < $this->maxConcurrency) {
                    $state['slots'][] = [
                        'id' => uniqid('', true),
                        'pid' => function_exists('getmypid') ? getmypid() : 0,
                        'ts' => microtime(true),
                    ];
                    $state['active'] = count($state['slots']);
                    $state['queued'] = max(0, $state['queued'] - 1);
                    $this->writeState($fp, $state);
                    flock($fp, LOCK_UN);
                    return $fp;
                }

                flock($fp, LOCK_UN);
            }
        }
    }

    /**
     * Releases an acquired permit slot.
     *
     * @param resource $fp
     */
    public function release($fp): void
    {
        if (!is_resource($fp)) {
            return;
        }

        while (true) {
            if (flock($fp, LOCK_EX)) {
                $content = stream_get_contents($fp, -1, 0);
                $state = $this->parseState($content);

                if (!empty($state['slots'])) {
                    array_shift($state['slots']);
                    $state['active'] = count($state['slots']);
                } else {
                    $state['active'] = max(0, $state['active'] - 1);
                }
                $this->writeState($fp, $state);

                flock($fp, LOCK_UN);
                fclose($fp);
                break;
            }
            usleep(1000);
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
