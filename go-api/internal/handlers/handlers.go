package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"tryon-api/internal/storage"
)

// Deps holds shared dependencies for handlers (DB pool, redis client, S3
// client, python-cv base URL, etc). Kept as a plain struct so it's trivial
// to wire real clients in later without changing handler signatures.
type Deps struct {
	PythonCVURL string
	S3          *storage.Client
	// DB    — needs a real driver (lib/pq or jackc/pgx). That's an
	//         external module, so wire it from your own machine where
	//         `go get` isn't blocked (this sandbox blocks proxy.golang.org).
	// Redis — same story; either a client library, or hand-rolled RESP
	//         if you want to stay fully dependency-free like storage/s3.go.
}

func Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// RequireBrandAPIKey is a stub auth middleware. Real version: look up the
// X-API-Key header against the `brands` table (or a Redis-cached copy of
// it), attach brand_id to the request context, reject on miss.
func RequireBrandAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "missing X-API-Key header",
			})
			return
		}
		// TODO: real lookup. For now any non-empty key passes so the
		// skeleton is runnable end to end without a seeded DB.
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// WithLogging is a minimal request logger. Swap for structured logging
// (zap/slog) once this leaves skeleton stage.
func WithLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
