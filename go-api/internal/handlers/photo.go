package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"tryon-api/internal/models"
)

// UploadCustomerPhoto is the ONE-TIME capture step.
//
// Flow:
//  1. Front-end SDK asks camera/file-picker for a photo, sends it here
//     as multipart/form-data.
//  2. We store the ORIGINAL photo in S3 — done below, fully wired.
//  3. We forward it to python-cv for face landmark extraction and get
//     back a compact embedding (measurements + keypoints), NOT a
//     description of the raw image — done below, fully wired.
//  4. We persist that embedding keyed by customer_id — STILL A TODO,
//     because it needs a Postgres/Redis driver, which is an external Go
//     module. This sandbox blocks proxy.golang.org, so `go get` can't
//     fetch one here. Run `go get github.com/jackc/pgx/v5` (or
//     lib/pq) from your own machine in Claude Code — that network path
//     isn't restricted there — then fill in the two TODOs marked below.
func (d *Deps) UploadCustomerPhoto(w http.ResponseWriter, r *http.Request) {
	customerID := r.PathValue("customerId")

	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB cap
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid upload"})
		return
	}
	file, header, err := r.FormFile("photo")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "photo field required"})
		return
	}
	defer file.Close()

	// Read once into memory (photos are capped at 10MB above) so we can
	// send the same bytes to both S3 and python-cv without the reader
	// being consumed by the first call.
	photoBytes, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read upload"})
		return
	}

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}

	// 1. Store the original photo in S3/MinIO. This is the "raw asset"
	//    copy — kept so we can re-run analysis later with a better model
	//    without asking the customer to upload again.
	s3Key := fmt.Sprintf("photos/%s/original.jpg", customerID)
	if d.S3 != nil {
		if err := d.S3.PutObject(s3Key, bytes.NewReader(photoBytes), int64(len(photoBytes)), contentType); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "failed to store photo: " + err.Error(),
			})
			return
		}
	}

	// 2. Forward to python-cv for face landmark extraction. This is the
	//    call that turns a raw photo into the reusable, storable
	//    embedding — the thing every future /tryon call actually needs.
	analysis, err := d.analyzePhoto(photoBytes, header.Filename, contentType)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": "face analysis failed: " + err.Error(),
		})
		return
	}
	if !analysis.Found {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
			"error":   "no_face_detected",
			"message": "Ask the customer for a clearer, front-facing photo.",
		})
		return
	}

	// 3. TODO: persist `analysis` (face_shape + landmarks) to Postgres,
	//    keyed by customerID, e.g.:
	//      INSERT INTO customer_photos (customer_id, s3_key, face_shape, landmarks)
	//      VALUES ($1, $2, $3, $4)
	//      ON CONFLICT (customer_id) DO UPDATE SET ...
	//    and cache the same row in Redis (SET customer:{id}:landmarks ...)
	//    so PhotoStatus and TryOn don't hit Postgres on every request.

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"customer_id": customerID,
		"status":      "photo_stored",
		"s3_key":      s3Key,
		"face_shape":  analysis.FaceShape,
		"note":        "S3 + python-cv wired; Postgres/Redis persistence still TODO (see comments)",
	})
}

// analyzePhoto forwards the photo bytes to the python-cv service's
// /analyze endpoint as multipart/form-data — mirrors exactly what the
// browser SDK does to us, just one hop further in.
func (d *Deps) analyzePhoto(photoBytes []byte, filename, contentType string) (*models.FaceAnalysis, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("photo", filename)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(photoBytes); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, d.PythonCVURL+"/analyze", &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusUnprocessableEntity {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("python-cv returned %s: %s", resp.Status, string(body))
	}

	var result models.FaceAnalysis
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// PhotoStatus lets the SDK check, before rendering the try-on button,
// whether this customer already has a stored photo — so returning
// customers never see the upload prompt again.
func (d *Deps) PhotoStatus(w http.ResponseWriter, r *http.Request) {
	customerID := r.PathValue("customerId")

	// TODO: real lookup — Redis first (fast path), Postgres fallback.
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"customer_id": customerID,
		"has_photo":   false,
	})
}
