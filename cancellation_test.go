package goloadguard_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Zihad-Bepari/GoLoadGuard"
)

// TestCancellationBeforeAcquisition verifies pre-check rejects already-canceled contexts.
func TestCancellationBeforeAcquisition(t *testing.T) {
	guard, err := goloadguard.New(goloadguard.Config{
		MaxConcurrency: 1,
		MaxQueueSize:   1,
	})
	if err != nil {
		t.Fatalf("unexpected init error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel prior to call

	executed := false
	err = guard.Do(ctx, func(ctx context.Context) error {
		executed = true
		return nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
	if executed {
		t.Fatal("callback executed despite pre-canceled context")
	}

	stats := guard.Stats()
	if stats.CurrentInFlight != 0 {
		t.Fatalf("in-flight permits leaked! got: %d", stats.CurrentInFlight)
	}
}

// TestCancellationWhileQueued verifies that waiters are cleanly unlinked when their context expires.
func TestCancellationWhileQueued(t *testing.T) {
	guard, err := goloadguard.New(goloadguard.Config{
		MaxConcurrency: 1,
		MaxQueueSize:   2,
	})
	if err != nil {
		t.Fatalf("unexpected init error: %v", err)
	}

	hold := make(chan struct{})
	started := make(chan struct{})

	// Occupy the only slot
	go func() {
		_ = guard.Do(context.Background(), func(ctx context.Context) error {
			close(started)
			<-hold
			return nil
		})
	}()

	<-started

	// Queued caller with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	executed := false
	err = guard.Do(ctx, func(ctx context.Context) error {
		executed = true
		return nil
	})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got: %v", err)
	}
	if executed {
		t.Fatal("callback executed after context deadline exceeded")
	}

	// Verify the waiter node was completely removed from the queue
	stats := guard.Stats()
	if stats.CurrentQueueSize != 0 {
		t.Fatalf("canceled waiter still in queue! queue depth: %d", stats.CurrentQueueSize)
	}
	if stats.CanceledRequests != 1 {
		t.Fatalf("expected 1 canceled request, got: %d", stats.CanceledRequests)
	}

	close(hold)
}

// TestPostGrantCancellationCheck verifies that if context cancels after slot is granted,
// the callback does NOT run and the permit is safely passed to the next waiter.
func TestPostGrantCancellationCheck(t *testing.T) {
	guard, err := goloadguard.New(goloadguard.Config{
		MaxConcurrency: 1,
		MaxQueueSize:   2,
	})
	if err != nil {
		t.Fatalf("unexpected init error: %v", err)
	}

	hold := make(chan struct{})
	started := make(chan struct{})

	// Occupy slot
	go func() {
		_ = guard.Do(context.Background(), func(ctx context.Context) error {
			close(started)
			<-hold
			return nil
		})
	}()

	<-started

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)

	executed1 := false
	var err1 error

	// Waiter 1: will cancel immediately upon release
	go func() {
		defer wg.Done()
		err1 = guard.Do(ctx, func(ctx context.Context) error {
			executed1 = true
			return nil
		})
	}()

	// Waiter 2: queued right behind Waiter 1
	time.Sleep(10 * time.Millisecond)
	wg.Add(1)

	executed2 := false
	var err2 error
	go func() {
		defer wg.Done()
		err2 = guard.Do(context.Background(), func(ctx context.Context) error {
			executed2 = true
			return nil
		})
	}()

	// Cancel Waiter 1's context right before freeing the slot
	time.Sleep(10 * time.Millisecond)
	cancel()
	close(hold)

	wg.Wait()

	if !errors.Is(err1, context.Canceled) {
		t.Fatalf("expected Waiter 1 to get context.Canceled, got: %v", err1)
	}
	if executed1 {
		t.Fatal("Waiter 1 callback executed despite cancellation")
	}

	// Waiter 2 MUST have received the transferred permit and executed
	if err2 != nil {
		t.Fatalf("Waiter 2 unexpected error: %v", err2)
	}
	if !executed2 {
		t.Fatal("Waiter 2 failed to execute after permit transfer")
	}

	stats := guard.Stats()
	if stats.CurrentInFlight != 0 {
		t.Fatalf("permit leaked! in-flight: %d", stats.CurrentInFlight)
	}
}
