package goloadguard

// Config defines the concurrency limits and queue parameters for Guard.
type Config struct {
	// MaxConcurrency specifies the maximum number of concurrent operations allowed.
	// Must be greater than 0.
	MaxConcurrency int

	// MaxQueueSize specifies the maximum number of callers that can wait in the FIFO queue.
	// Must be greater than or equal to 0.
	// A value of 0 means no waiting queue: if all concurrency slots are full,
	// incoming requests are immediately rejected with ErrOverloaded.
	MaxQueueSize int
}

// Validate checks whether the configuration values conform to requirements.
func (c Config) Validate() error {
	if c.MaxConcurrency <= 0 {
		return ErrInvalidConfig
	}
	if c.MaxQueueSize < 0 {
		return ErrInvalidConfig
	}
	return nil
}
