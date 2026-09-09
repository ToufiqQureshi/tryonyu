package handlers

import (
	"encoding/json"
	"net/http"
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
	ResultImageURL string `json:"result_image_url"`
	LatencyMs      int    `json:"latency_ms"`
	Method         string `json:"method"` // "landmark_overlay" | "diffusion"
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

	// TODO:
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
