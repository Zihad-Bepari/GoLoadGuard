package goloadguard

import "errors"

var (
	// ErrOverloaded is returned when both active capacity and the waiting queue are full.
	ErrOverloaded = errors.New("goloadguard: system overloaded, capacity and queue full")

	// ErrClosed is returned when an operation is attempted on a closed guard,
	// or to queued waiters when Close is invoked.
	ErrClosed = errors.New("goloadguard: guard is closed")

	// ErrInvalidConfig is returned when configuration parameters fail validation.
	ErrInvalidConfig = errors.New("goloadguard: invalid configuration parameters")

	// ErrInvalidContext is returned when a nil context is passed to Do.
	ErrInvalidContext = errors.New("goloadguard: context cannot be nil")

	// ErrNilCallback is returned when a nil callback function is passed to Do.
	ErrNilCallback = errors.New("goloadguard: callback function cannot be nil")
)
