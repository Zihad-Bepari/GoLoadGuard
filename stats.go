package goloadguard

// Stats provides a point-in-time snapshot of the Guard metrics.
//
// All counters represent cumulative totals since Guard creation, while
// CurrentInFlight and CurrentQueueSize represent the instantaneous state.
type Stats struct {
	// CurrentInFlight is the number of permits currently active or granted.
	CurrentInFlight int64

	// CurrentQueueSize is the number of callers currently waiting in the FIFO queue.
	CurrentQueueSize int64

	// AcceptedRequests is the cumulative number of successful permit acquisitions.
	AcceptedRequests int64

	// RejectedRequests is the cumulative number of requests immediately rejected
	// due to both capacity and queue being full (ErrOverloaded).
	RejectedRequests int64

	// CanceledRequests is the cumulative number of requests canceled while waiting
	// in the queue due to caller context cancellation/deadline.
	CanceledRequests int64

	// ClosedRejections is the cumulative number of requests rejected because the
	// guard is closed (either at admission or unblocked from queue upon Close).
	ClosedRejections int64
}
