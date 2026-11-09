package main

import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/Zihad-Bepari/GoLoadGuard"
	"github.com/Zihad-Bepari/GoLoadGuard/httpguard"
)

func main() {
	// Guard allowing at most 5 concurrent requests and up to 10 queued callers.
	guard, err := goloadguard.New(goloadguard.Config{
		MaxConcurrency: 5,
		MaxQueueSize:   10,
	})
	if err != nil {
		log.Fatalf("failed to create guard: %v", err)
	}
	defer guard.Close()

	var counter int64

	mux := http.NewServeMux()
	mux.HandleFunc("/heavy-task", func(w http.ResponseWriter, r *http.Request) {
		id := atomic.AddInt64(&counter, 1)

		// Simulate intensive I/O or database work
		select {
		case <-time.After(100 * time.Millisecond):
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "Task %d finished successfully\n", id)
		case <-r.Context().Done():
			// Client canceled or timed out
			return
		}
	})

	// Wrap entire mux with GoLoadGuard HTTP middleware
	handler := httpguard.Middleware(guard, httpguard.Options{
		OverloadStatusCode: http.StatusServiceUnavailable, // 503
		RetryAfterSeconds:  5,                           // Retry-After: 5
	})(mux)

	fmt.Println("Protected HTTP server running on :8080...")
	if err := http.ListenAndServe(":8080", handler); err != nil {
		log.Fatal(err)
	}
}
