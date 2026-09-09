package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"tryon-api/internal/handlers"
	"tryon-api/internal/storage"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	pythonCVURL := os.Getenv("PYTHON_CV_URL")
	if pythonCVURL == "" {
		pythonCVURL = "http://localhost:8000"
	}

	s3Client := storage.NewClient(
		envOr("S3_ENDPOINT", "http://localhost:9000"),
		envOr("S3_REGION", "us-east-1"), // MinIO ignores region in practice; AWS needs the real one
		envOr("S3_BUCKET", "tryon-photos"),
		os.Getenv("S3_ACCESS_KEY"),
		os.Getenv("S3_SECRET_KEY"),
	)

	deps := &handlers.Deps{
		PythonCVURL: pythonCVURL,
		S3:          s3Client,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", handlers.Health)

	// Brand-authenticated routes. Using Go 1.22's method+path patterns
	// directly on the stdlib mux — no third-party router needed, keeps
	// the skeleton dependency-free and easy to vendor/audit.
	mux.Handle("POST /api/v1/customers/{customerId}/photo",
		handlers.RequireBrandAPIKey(http.HandlerFunc(deps.UploadCustomerPhoto)))
	mux.Handle("GET /api/v1/customers/{customerId}/photo-status",
		handlers.RequireBrandAPIKey(http.HandlerFunc(deps.PhotoStatus)))
	mux.Handle("POST /api/v1/tryon",
		handlers.RequireBrandAPIKey(http.HandlerFunc(deps.TryOn)))
	mux.Handle("POST /api/v1/recommend",
		handlers.RequireBrandAPIKey(http.HandlerFunc(deps.Recommend)))

	handler := handlers.WithLogging(mux)

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	log.Printf("go-api listening on :%s (python-cv at %s)", port, pythonCVURL)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
