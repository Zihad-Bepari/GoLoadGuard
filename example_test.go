package goloadguard_test

import (
	"context"
	"fmt"
	"time"

	"github.com/Zihad-Bepari/GoLoadGuard"
)

// ExampleGuard_Do demonstrates protecting a critical section with concurrency limits.
func ExampleGuard_Do() {
	// Initialize a guard allowing at most 2 concurrent executions and up to 5 queued callers.
	guard, err := goloadguard.New(goloadguard.Config{
		MaxConcurrency: 2,
		MaxQueueSize:   5,
	})
	if err != nil {
		panic(err)
	}
	defer guard.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Execute a protected operation
	err = guard.Do(ctx, func(ctx context.Context) error {
		// Simulate database query or computation
		return nil
	})
	if err != nil {
		fmt.Printf("Workload failed: %v\n", err)
		return
	}

	fmt.Println("Workload completed successfully")
	// Output:
	// Workload completed successfully
}

// ExampleGuard_Stats demonstrates reading runtime metrics snapshot.
func ExampleGuard_Stats() {
	guard, err := goloadguard.New(goloadguard.Config{
		MaxConcurrency: 5,
		MaxQueueSize:   10,
	})
	if err != nil {
		panic(err)
	}
	defer guard.Close()

	_ = guard.Do(context.Background(), func(ctx context.Context) error {
		return nil
	})

	stats := guard.Stats()
	fmt.Printf("Accepted: %d, InFlight: %d\n", stats.AcceptedRequests, stats.CurrentInFlight)
	// Output:
	// Accepted: 1, InFlight: 0
}
