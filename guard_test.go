package goloadguard_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goloadguard/goloadguard"
)

// TestConfigValidation verifies that invalid configurations return ErrInvalidConfig.
func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		cfg         goloadguard.Config
		expectError bool
	}{
		{
			name:        "Valid config",
			cfg:         goloadguard.Config{MaxConcurrency: 5, MaxQueueSize: 10},
			expectError: false,
		},
		{
			name:        "Valid config with zero queue",
			cfg:         goloadguard.Config{MaxConcurrency: 1, MaxQueueSize: 0},
			expectError: false,
		},
		{
			name:        "Invalid concurrency (0)",
			cfg:         goloadguard.Config{MaxConcurrency: 0, MaxQueueSize: 10},
			expectError: true,
		},
		{
			name:        "Invalid concurrency (<0)",
			cfg:         goloadguard.Config{MaxConcurrency: -1, MaxQueueSize: 10},
			expectError: true,
		},
		{
			name:        "Invalid queue size (<0)",
			cfg:         goloadguard.Config{MaxConcurrency: 5, MaxQueueSize: -1},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := goloadguard.New(tt.cfg)
			if (err != nil) != tt.expectError {
				t.Fatalf("expected error: %v, got: %v", tt.expectError, err)
			}
			if tt.expectError && !errors.Is(err, goloadguard.ErrInvalidConfig) {
				t.Fatalf("expected ErrInvalidConfig, got: %v", err)
			}
		})
	}
}

// TestNilContextAndCallback verifies defensive validation for nil arguments.
func TestNilContextAndCallback(t *testing.T) {
	guard, err := goloadguard.New(goloadguard.Config{MaxConcurrency: 1, MaxQueueSize: 0})
	if err != nil {
		t.Fatalf("unexpected init error: %v", err)
	}

	// Nil context
	err = guard.Do(nil, func(ctx context.Context) error { return nil })
	if !errors.Is(err, goloadguard.ErrInvalidContext) {
		t.Fatalf("expected ErrInvalidContext, got: %v", err)
	}

	// Nil callback
	err = guard.Do(context.Background(), nil)
	if !errors.Is(err, goloadguard.ErrNilCallback) {
		t.Fatalf("expected ErrNilCallback, got: %v", err)
	}
}

// TestStrictConcurrencyLimit ensures active executions never exceed MaxConcurrency.
func TestStrictConcurrencyLimit(t *testing.T) {
	const maxConcurrency = 3
	const totalCallers = 20

	guard, err := goloadguard.New(goloadguard.Config{
		MaxConcurrency: maxConcurrency,
		MaxQueueSize:   totalCallers,
	})
	if err != nil {
		t.Fatalf("unexpected init error: %v", err)
	}

	var currentActive int64
	var maxObserved int64
	var wg sync.WaitGroup

	for i := 0; i < totalCallers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := guard.Do(context.Background(), func(ctx context.Context) error {
				val := atomic.AddInt64(&currentActive, 1)

				// Update peak concurrency observed
				for {
					peak := atomic.LoadInt64(&maxObserved)
					if val <= peak || atomic.CompareAndSwapInt64(&maxObserved, peak, val) {
						break
					}
				}

				// Artificial work
				time.Sleep(20 * time.Millisecond)
				atomic.AddInt64(&currentActive, -1)
				return nil
			})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}

	wg.Wait()

	if maxObserved > maxConcurrency {
		t.Fatalf("concurrency limit breached! allowed: %d, observed: %d", maxConcurrency, maxObserved)
	}

	stats := guard.Stats()
	if stats.CurrentInFlight != 0 {
		t.Fatalf("in-flight permits leaked! expected 0, got %d", stats.CurrentInFlight)
	}
	if stats.AcceptedRequests != totalCallers {
		t.Fatalf("expected %d accepted requests, got %d", totalCallers, stats.AcceptedRequests)
	}
}

// TestImmediateOverloadRejection tests that requests are immediately rejected when queue is full.
func TestImmediateOverloadRejection(t *testing.T) {
	guard, err := goloadguard.New(goloadguard.Config{
		MaxConcurrency: 1,
		MaxQueueSize:   0, // zero-queue mode
	})
	if err != nil {
		t.Fatalf("unexpected init error: %v", err)
	}

	hold := make(chan struct{})
	entered := make(chan struct{})

	// Slot occupier
	go func() {
		_ = guard.Do(context.Background(), func(ctx context.Context) error {
			close(entered)
			<-hold
			return nil
		})
	}()

	<-entered

	// This call must be rejected immediately
	err = guard.Do(context.Background(), func(ctx context.Context) error {
		return nil
	})

	if !errors.Is(err, goloadguard.ErrOverloaded) {
		t.Fatalf("expected ErrOverloaded, got: %v", err)
	}

	close(hold)
}

// TestFIFOOrder ensures queued callers receive permits in exact arrival order.
func TestFIFOOrder(t *testing.T) {
	guard, err := goloadguard.New(goloadguard.Config{
		MaxConcurrency: 1,
		MaxQueueSize:   5,
	})
	if err != nil {
		t.Fatalf("unexpected init error: %v", err)
	}

	hold := make(chan struct{})
	activeStarted := make(chan struct{})

	// Occupy the only slot
	go func() {
		_ = guard.Do(context.Background(), func(ctx context.Context) error {
			close(activeStarted)
			<-hold
			return nil
		})
	}()

	<-activeStarted

	const waiterCount = 3
	orderChan := make(chan int, waiterCount)
	var wg sync.WaitGroup

	for i := 1; i <= waiterCount; i++ {
		id := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = guard.Do(context.Background(), func(ctx context.Context) error {
				orderChan <- id
				return nil
			})
		}()
		// Minor synchronization to ensure sequential queuing
		time.Sleep(10 * time.Millisecond)
	}

	// Release the initial slot
	close(hold)
	wg.Wait()
	close(orderChan)

	expectedID := 1
	for id := range orderChan {
		if id != expectedID {
			t.Fatalf("FIFO order violation! expected ID %d, got %d", expectedID, id)
		}
		expectedID++
	}
}

// TestPanicPermitRelease ensures that callback panics safely release the permit.
func TestPanicPermitRelease(t *testing.T) {
	guard, err := goloadguard.New(goloadguard.Config{
		MaxConcurrency: 1,
		MaxQueueSize:   0,
	})
	if err != nil {
		t.Fatalf("unexpected init error: %v", err)
	}

	// Trigger a panic in protected callback
	func() {
		defer func() {
			_ = recover()
		}()
		_ = guard.Do(context.Background(), func(ctx context.Context) error {
			panic("simulated fatal error")
		})
	}()

	// The permit must have been cleanly released. A subsequent call must succeed.
	executed := false
	err = guard.Do(context.Background(), func(ctx context.Context) error {
		executed = true
		return nil
	})

	if err != nil {
		t.Fatalf("subsequent call failed after panic: %v", err)
	}
	if !executed {
		t.Fatal("subsequent call did not execute")
	}

	stats := guard.Stats()
	if stats.CurrentInFlight != 0 {
		t.Fatalf("permit leaked on panic! in-flight: %d", stats.CurrentInFlight)
	}
}

// TestCloseBehavior ensures that Close rejects queued waiters and halts new acquisitions.
func TestCloseBehavior(t *testing.T) {
	guard, err := goloadguard.New(goloadguard.Config{
		MaxConcurrency: 1,
		MaxQueueSize:   2,
	})
	if err != nil {
		t.Fatalf("unexpected init error: %v", err)
	}

	hold := make(chan struct{})
	started := make(chan struct{})

	// Running caller
	go func() {
		_ = guard.Do(context.Background(), func(ctx context.Context) error {
			close(started)
			<-hold
			return nil
		})
	}()

	<-started

	// Queued caller
	queuedErrChan := make(chan error, 1)
	go func() {
		err := guard.Do(context.Background(), func(ctx context.Context) error {
			return nil
		})
		queuedErrChan <- err
	}()

	// Allow goroutine to queue up
	time.Sleep(10 * time.Millisecond)

	// Close the guard
	guard.Close()

	// Queued caller must receive ErrClosed
	queuedErr := <-queuedErrChan
	if !errors.Is(queuedErr, goloadguard.ErrClosed) {
		t.Fatalf("expected ErrClosed for queued waiter, got: %v", queuedErr)
	}

	// New caller must receive ErrClosed immediately
	newErr := guard.Do(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if !errors.Is(newErr, goloadguard.ErrClosed) {
		t.Fatalf("expected ErrClosed for new caller, got: %v", newErr)
	}

	// Running caller completes normally
	close(hold)
}
