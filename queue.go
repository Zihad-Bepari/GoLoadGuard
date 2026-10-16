package goloadguard

// waiterState defines the terminal states of a queued waiter.
type waiterState int

const (
	stateWaiting waiterState = iota
	stateGranted
	stateCanceled
	stateClosed
)

// waiter represents a caller blocked waiting for a permit in the FIFO queue.
type waiter struct {
	state waiterState
	ready chan struct{}
	prev  *waiter
	next  *waiter
}

func newWaiter() *waiter {
	return &waiter{
		state: stateWaiting,
		ready: make(chan struct{}),
	}
}

// waiterQueue is an explicit doubly-linked list for FIFO permit grants and O(1) removal.
type waiterQueue struct {
	head *waiter
	tail *waiter
	len  int
}

func newWaiterQueue() *waiterQueue {
	return &waiterQueue{}
}

// Push appends a waiter to the tail of the FIFO queue.
func (q *waiterQueue) Push(w *waiter) {
	w.prev = q.tail
	w.next = nil
	if q.tail != nil {
		q.tail.next = w
	} else {
		q.head = w
	}
	q.tail = w
	q.len++
}

// PopHead extracts the head waiter from the FIFO queue.
func (q *waiterQueue) PopHead() *waiter {
	if q.head == nil {
		return nil
	}
	w := q.head
	q.Remove(w)
	return w
}

// Remove unlinks a specific waiter node from the doubly-linked list in O(1) time.
func (q *waiterQueue) Remove(w *waiter) {
	if w.prev != nil {
		w.prev.next = w.next
	} else {
		q.head = w.next
	}
	if w.next != nil {
		w.next.prev = w.prev
	} else {
		q.tail = w.prev
	}
	w.prev = nil
	w.next = nil
	q.len--
}

// Len returns the number of waiters currently queued.
func (q *waiterQueue) Len() int {
	return q.len
}
