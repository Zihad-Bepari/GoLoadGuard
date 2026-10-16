// Package goloadguard provides production-grade, in-memory concurrency control
// and bounded FIFO backpressure protection for Go backend workloads.
//
// The core package is zero-dependency and relies exclusively on the Go standard library.
//
// Basic usage:
//
//	guard, err := goloadguard.New(goloadguard.Config{
//	    MaxConcurrency: 10,
//	    MaxQueueSize:   50,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer guard.Close()
//
//	err = guard.Do(ctx, func(ctx context.Context) error {
//	    return processJob(ctx)
//	})
package goloadguard
