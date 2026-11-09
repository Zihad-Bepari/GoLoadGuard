package goloadguard_test

import (
	"context"
	"testing"

	"github.com/Zihad-Bepari/GoLoadGuard"
)

// BenchmarkUncontended measures the latency and allocations of uncontended Do calls.
func BenchmarkUncontended(b *testing.B) {
	guard, err := goloadguard.New(goloadguard.Config{
		MaxConcurrency: 100,
		MaxQueueSize:   100,
	})
	if err != nil {
		b.Fatalf("init error: %v", err)
	}

	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = guard.Do(ctx, func(ctx context.Context) error {
			return nil
		})
	}
}

// BenchmarkConcurrentContention measures throughput under high parallel contention.
func BenchmarkConcurrentContention(b *testing.B) {
	guard, err := goloadguard.New(goloadguard.Config{
		MaxConcurrency: 10,
		MaxQueueSize:   1000,
	})
	if err != nil {
		b.Fatalf("init error: %v", err)
	}

	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = guard.Do(ctx, func(ctx context.Context) error {
				return nil
			})
		}
	})
}
