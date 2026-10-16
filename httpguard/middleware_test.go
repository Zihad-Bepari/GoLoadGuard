package httpguard_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/goloadguard/goloadguard"
	"github.com/goloadguard/goloadguard/httpguard"
)

func TestHTTPMiddlewareOverload(t *testing.T) {
	guard, err := goloadguard.New(goloadguard.Config{
		MaxConcurrency: 1,
		MaxQueueSize:   0, // reject immediately when 1 in-flight
	})
	if err != nil {
		t.Fatalf("unexpected init error: %v", err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	protected := httpguard.Middleware(guard, httpguard.Options{
		OverloadStatusCode: http.StatusServiceUnavailable,
		RetryAfterSeconds:  5,
	})(handler)

	var wg sync.WaitGroup
	wg.Add(2)

	var resp1, resp2 *httptest.ResponseRecorder

	// Request 1: gets the permit
	go func() {
		defer wg.Done()
		req := httptest.NewRequest("GET", "/test", nil)
		resp1 = httptest.NewRecorder()
		protected.ServeHTTP(resp1, req)
	}()

	// Brief delay to ensure request 1 enters
	time.Sleep(10 * time.Millisecond)

	// Request 2: must be rejected with 503 and Retry-After: 5
	go func() {
		defer wg.Done()
		req := httptest.NewRequest("GET", "/test", nil)
		resp2 = httptest.NewRecorder()
		protected.ServeHTTP(resp2, req)
	}()

	wg.Wait()

	if resp1.Code != http.StatusOK {
		t.Errorf("request 1 expected status 200, got: %d", resp1.Code)
	}

	if resp2.Code != http.StatusServiceUnavailable {
		t.Errorf("request 2 expected status 503, got: %d", resp2.Code)
	}

	retryAfter := resp2.Header().Get("Retry-After")
	if retryAfter != "5" {
		t.Errorf("expected Retry-After: 5, got: %s", retryAfter)
	}
}
