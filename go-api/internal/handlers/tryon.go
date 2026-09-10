package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type TryOnRequest struct {
	CustomerID string `json:"customer_id"`
	ProductID  string `json:"product_id"`
	// ProductImageURL is the brand's own catalog image — we never ask the
	// brand to re-upload anything, we just fetch what they already host.
	ProductImageURL string `json:"product_image_url"`
	Category        string `json:"category"` // "eyewear" | "clothing_top" | ...
}

type TryOnResponse struct {
	ResultImageURL string `json:"result_image_url,omitempty"`
	LatencyMs      int    `json:"latency_ms,omitempty"`
	Method         string `json:"method,omitempty"` // "landmark_overlay" | "diffusion"

	// Set instead of the above for category == "clothing_*", since
	// generation is async (15-60s) — see TryOnStatus below for polling.
	JobID  string `json:"job_id,omitempty"`
	Status string `json:"status,omitempty"` // "processing" | "done" | "failed"
}

type clothingJobRequest struct {
	PersonPhotoURL  string `json:"person_photo_url"`
	ProductImageURL string `json:"product_image_url"`
}

type clothingJobResponse struct {
	JobID  string `json:"job_id"`
	Status string `json:"status"`
}

// TryOn is the hot path: called every time a customer clicks "try on" on
// a product page. Must be fast and cheap because it runs at product-view
// volume, not at purchase volume.
//
// Routing logic (the actual cost-control decision from our research):
//   - category == "eyewear"        -> landmark_overlay, CPU only, ~instant
//   - category == clothing & phase1 -> landmark_overlay (rough silhouette fit)
//   - category == clothing & phase2 -> GPU diffusion worker queue (async),
//     only enabled once volume justifies the GPU cost
//
// This handler stays a thin orchestrator: fetch stored customer embedding,
// call python-cv, return URL. No business logic about *which* model runs
// lives here — python-cv decides based on the category flag we pass it.
func (d *Deps) TryOn(w http.ResponseWriter, r *http.Request) {
	var req TryOnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if req.CustomerID == "" || req.ProductImageURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "customer_id and product_image_url are required",
		})
		return
	}

	if strings.HasPrefix(req.Category, "clothing") {
		d.tryOnClothing(w, req)
		return
	}

	// TODO (eyewear, phase 4 — see ROADMAP.md):
	//   1. look up stored embedding/landmarks for req.CustomerID
	//      (Redis first, Postgres fallback) — 404 if PhotoStatus was
	//      never true, telling the SDK to prompt for a photo first
	//   2. POST { embedding, product_image_url, category } to
	//      d.PythonCVURL + "/tryon"
	//   3. store result in S3, return a CDN URL

	writeJSON(w, http.StatusOK, TryOnResponse{
		ResultImageURL: "https://cdn.example.com/results/stub.jpg",
		LatencyMs:      0,
		Method:         "landmark_overlay",
	})
}

// tryOnClothing kicks off the async diffusion job in python-cv and
// returns the job id immediately — the SDK polls TryOnStatus instead of
// blocking, since real generation takes 15-60s (see ROADMAP.md Phase 2).
//
// It does NOT re-fetch or re-upload the customer's photo: it derives the
// same S3 key UploadCustomerPhoto already wrote to
// (photos/{customerId}/original.jpg) and presigns a short-lived read URL
// for python-cv to fetch — the "photo once, reuse everywhere" moat holds
// for clothing exactly like it does for eyewear.
func (d *Deps) tryOnClothing(w http.ResponseWriter, req TryOnRequest) {
	if d.S3 == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "storage not configured"})
		return
	}

	s3Key := fmt.Sprintf("photos/%s/original.jpg", req.CustomerID)
	photoURL, err := d.S3.PresignGetURL(s3Key, 15*time.Minute)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to presign stored photo: " + err.Error(),
		})
		return
	}

	jobReq := clothingJobRequest{
		PersonPhotoURL:  photoURL,
		ProductImageURL: req.ProductImageURL,
	}
	body, err := json.Marshal(jobReq)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to encode job request"})
		return
	}

	resp, err := http.Post(d.PythonCVURL+"/tryon/clothing", "application/json", bytes.NewReader(body))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "python-cv unreachable: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	var job clothingJobResponse
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "invalid response from python-cv"})
		return
	}

	writeJSON(w, http.StatusAccepted, TryOnResponse{JobID: job.JobID, Status: job.Status})
}

// TryOnStatus lets the SDK poll a clothing try-on job started by TryOn.
// Thin proxy to python-cv's job store (see python-cv/jobs.py) — no
// business logic lives here on purpose, matching TryOn's "thin
// orchestrator" role.
func (d *Deps) TryOnStatus(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobId")

	resp, err := http.Get(d.PythonCVURL + "/tryon/clothing/" + jobID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "python-cv unreachable: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to read python-cv response"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

type RecommendRequest struct {
	CustomerID string `json:"customer_id"`
	BrandID    string `json:"brand_id"`
}

type RecommendResponse struct {
	FaceShape       string   `json:"face_shape"`
	RecommendedSKUs []string `json:"recommended_skus"`
}

// Recommend is the eyewear-specific wedge feature: given the customer's
// stored face landmarks, classify face shape and return SKUs from the
// BRAND'S OWN catalog (never a generic list) that suit that shape.
func (d *Deps) Recommend(w http.ResponseWriter, r *http.Request) {
	var req RecommendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}

	// TODO:
	//   1. fetch stored landmarks for req.CustomerID
	//   2. run face-shape heuristic (ratios from landmarks; see
	//      python-cv/face_analysis.py) or call python-cv/classify
	//   3. join against brand's catalog table filtered by suited shapes

	writeJSON(w, http.StatusOK, RecommendResponse{
		FaceShape:       "oval",
		RecommendedSKUs: []string{"SKU-STUB-1", "SKU-STUB-2"},
	})
}
