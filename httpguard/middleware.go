package httpguard

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/Zihad-Bepari/GoLoadGuard"
)

// Options configures the HTTP middleware behavior.
type Options struct {
	// OverloadStatusCode specifies the HTTP status returned when the system is overloaded.
	// Defaults to http.StatusServiceUnavailable (503).
	OverloadStatusCode int

	// RetryAfterSeconds optionally sets the "Retry-After" header in seconds when overloaded.
	// Set to 0 to omit the header.
	RetryAfterSeconds int
}

// Middleware wraps an http.Handler with GoLoadGuard protection.
func Middleware(guard *goloadguard.Guard, opts ...Options) func(http.Handler) http.Handler {
	cfg := Options{
		OverloadStatusCode: http.StatusServiceUnavailable,
	}
	if len(opts) > 0 {
		if opts[0].OverloadStatusCode != 0 {
			cfg.OverloadStatusCode = opts[0].OverloadStatusCode
		}
		cfg.RetryAfterSeconds = opts[0].RetryAfterSeconds
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			err := guard.Do(r.Context(), func(ctx context.Context) error {
				// Execute the protected HTTP handler with the request context
				next.ServeHTTP(w, r.WithContext(ctx))
				return nil
			})

			if err != nil {
				// System is overloaded or guard is closed
				if errors.Is(err, goloadguard.ErrOverloaded) || errors.Is(err, goloadguard.ErrClosed) {
					if cfg.RetryAfterSeconds > 0 {
						w.Header().Set("Retry-After", strconv.Itoa(cfg.RetryAfterSeconds))
					}
					http.Error(w, http.StatusText(cfg.OverloadStatusCode), cfg.OverloadStatusCode)
					return
				}

				// If request context canceled or timed out, do not attempt to write headers
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return
				}
			}
		})
	}
}
