package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/goloadguard/goloadguard"
)

type Job struct {
	ID int
}

func main() {
	// Guard allowing at most 3 workers simultaneously running with queue capacity 10
	guard, err := goloadguard.New(goloadguard.Config{
		MaxConcurrency: 3,
		MaxQueueSize:   10,
	})
	if err != nil {
		log.Fatalf("failed to create guard: %v", err)
	}
	defer guard.Close()

	totalJobs := 15
	var wg sync.WaitGroup

	for i := 1; i <= totalJobs; i++ {
		job := Job{ID: i}
		wg.Add(1)

		go func(j Job) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			err := guard.Do(ctx, func(ctx context.Context) error {
				fmt.Printf("[Worker] Processing Job %d (Active slots busy)...\n", j.ID)
				time.Sleep(50 * time.Millisecond)
				fmt.Printf("[Worker] Completed Job %d\n", j.ID)
				return nil
			})

			if err != nil {
				fmt.Printf("[Worker] Job %d rejected or failed: %v\n", j.ID, err)
			}
		}(job)
	}

	wg.Wait()

	stats := guard.Stats()
	fmt.Printf("\nFinal Stats: Accepted: %d, Overload Rejections: %d\n",
		stats.AcceptedRequests, stats.RejectedRequests)
}
