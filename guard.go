package goloadguard

import (
	"context"
	"sync"
)

// Guard coordinates concurrent execution limits and backpressure queueing.
type Guard struct {
	cfg Config

	mu     sync.Mutex
	closed bool

	// inFlight counts the total number of permits currently in use or granted to waiters.
	inFlight int
	waiters  *waiterQueue
	stats    Stats
}

// New creates and validates a new Guard with the provided Config.
func New(cfg Config) (*Guard, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Guard{
		cfg:     cfg,
		waiters: newWaiterQueue(),
	}, nil
}

// Do executes fn within the concurrency guard.
//
// Concurrency guarantees:
// 1. If active permits < MaxConcurrency and queue is empty, execution starts immediately.
// 2. If slots are busy and queue < MaxQueueSize, caller waits in FIFO order.
// 3. If slots and queue are both full, ErrOverloaded is returned immediately without waiting.
// 4. If guard is closed, ErrClosed is returned.
// 5. If caller context is canceled before or during wait, caller is removed from queue.
// 6. Post-grant check: If context cancels after grant but before fn execution,
//    the acquired permit is reliably released/transferred without executing fn.
// 7. Panics in fn naturally propagate while ensuring permit release via defer.
func (g *Guard) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	if ctx == nil {
		return ErrInvalidContext
	}
	if fn == nil {
		return ErrNilCallback
	}

	// 1. Pre-check: if caller context is already canceled, do not attempt acquisition
	if err := ctx.Err(); err != nil {
		return err
	}

	// 2. Acquire a permit (or wait in FIFO queue)
	if err := g.acquire(ctx); err != nil {
		return err
	}

	// Ensure the permit is released exactly once, even if fn panics or returns error
	defer g.release()

	// 3. Post-grant check: verify context did not cancel right after grant
	if err := ctx.Err(); err != nil {
		return err
	}

	// 4. Execute the protected workload
	return fn(ctx)
}

// acquire attempts to obtain an execution slot or queues the caller.
func (g *Guard) acquire(ctx context.Context) error {
	g.mu.Lock()

	// Check if guard is closed
	if g.closed {
		g.stats.ClosedRejections++
		g.mu.Unlock()
		return ErrClosed
	}

	// Check if caller context is already canceled
	if err := ctx.Err(); err != nil {
		g.mu.Unlock()
		return err
	}

	// Fast path: Immediate admission if active slots are below MaxConcurrency
	// and no prior waiters are pending in queue (preserves FIFO fairness).
	if g.inFlight < g.cfg.MaxConcurrency && g.waiters.Len() == 0 {
		g.inFlight++
		g.stats.AcceptedRequests++
		g.mu.Unlock()
		return nil
	}

	// Check bounded queue capacity
	if g.waiters.Len() >= g.cfg.MaxQueueSize {
		g.stats.RejectedRequests++
		g.mu.Unlock()
		return ErrOverloaded
	}

	// Enqueue waiter
	w := newWaiter()
	g.waiters.Push(w)
	g.mu.Unlock()

	// Wait for slot grant, cancellation, or guard closure
	select {
	case <-w.ready:
		// Slot has been granted or guard was closed.
		// Check the terminal state under lock.
		g.mu.Lock()
		state := w.state
		g.mu.Unlock()

		if state == stateClosed {
			return ErrClosed
		}
		// Permit successfully granted
		return nil

	case <-ctx.Done():
		// Context was canceled while blocked in queue.
		// Resolve the grant-versus-cancellation race under the mutex.
		g.mu.Lock()
		switch w.state {
		case stateWaiting:
			// Cancellation won the race.
			w.state = stateCanceled
			g.waiters.Remove(w)
			g.stats.CanceledRequests++
			g.mu.Unlock()
			return ctx.Err()

		case stateGranted:
			// Grant won the race right as context was canceled.
			// The caller now owns this permit.
			// Release the permit back or transfer it to the next waiter.
			g.mu.Unlock()
			g.release()
			return ctx.Err()

		case stateClosed:
			g.mu.Unlock()
			return ErrClosed

		default:
			g.mu.Unlock()
			return ctx.Err()
		}
	}
}

// release frees an active slot and hands it off to the next queued waiter in FIFO order.
func (g *Guard) release() {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Attempt to hand off permit to the head waiter
	for g.waiters.Len() > 0 {
		w := g.waiters.PopHead()
		if w.state == stateWaiting {
			w.state = stateGranted
			g.stats.AcceptedRequests++
			close(w.ready)
			return // Permit successfully transferred, inFlight remains constant
		}
		// If waiter was already canceled/closed, continue to next waiter
	}

	// No pending waiters; decrement active in-flight count
	g.inFlight--
}

// Close closes the guard, preventing new admissions and unblocking all queued waiters with ErrClosed.
// Active in-flight operations complete normally. Close is idempotent.
func (g *Guard) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.closed {
		return
	}
	g.closed = true

	// Drain all queued waiters and transition them to stateClosed
	for g.waiters.Len() > 0 {
		w := g.waiters.PopHead()
		if w.state == stateWaiting {
			w.state = stateClosed
			g.stats.ClosedRejections++
			close(w.ready)
		}
	}
}

// Stats returns a race-safe snapshot of Guard metrics.
func (g *Guard) Stats() Stats {
	g.mu.Lock()
	defer g.mu.Unlock()

	s := g.stats
	s.CurrentInFlight = int64(g.inFlight)
	s.CurrentQueueSize = int64(g.waiters.Len())
	return s
}
