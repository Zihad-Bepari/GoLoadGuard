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
func (g *Guard) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	if ctx == nil {
		return ErrInvalidContext
	}
	if fn == nil {
		return ErrNilCallback
	}

	// check context cancellation
	if err := ctx.Err(); err != nil {
		return err
	}

	// acquire permit
	if err := g.acquire(ctx); err != nil {
		return err
	}
	defer g.release()

	// verify context cancellation after grant
	if err := ctx.Err(); err != nil {
		return err
	}

	return fn(ctx)
}

// acquire attempts to obtain an execution slot or queues the caller.
func (g *Guard) acquire(ctx context.Context) error {
	g.mu.Lock()

	if g.closed {
		g.stats.ClosedRejections++
		g.mu.Unlock()
		return ErrClosed
	}

	if err := ctx.Err(); err != nil {
		g.mu.Unlock()
		return err
	}

	// fast path: immediate admission
	if g.inFlight < g.cfg.MaxConcurrency && g.waiters.Len() == 0 {
		g.inFlight++
		g.stats.AcceptedRequests++
		g.mu.Unlock()
		return nil
	}

	// check queue capacity
	if g.waiters.Len() >= g.cfg.MaxQueueSize {
		g.stats.RejectedRequests++
		g.mu.Unlock()
		return ErrOverloaded
	}

	// enqueue waiter
	w := newWaiter()
	g.waiters.Push(w)
	g.mu.Unlock()

	// wait for slot or cancellation
	select {
	case <-w.ready:
		g.mu.Lock()
		state := w.state
		g.mu.Unlock()

		if state == stateClosed {
			return ErrClosed
		}
		return nil

	case <-ctx.Done():
		g.mu.Lock()
		switch w.state {
		case stateWaiting:
			w.state = stateCanceled
			g.waiters.Remove(w)
			g.stats.CanceledRequests++
			g.mu.Unlock()
			return ctx.Err()

		case stateGranted:
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
