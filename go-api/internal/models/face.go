package models

// FaceAnalysis mirrors the JSON shape returned by python-cv's /analyze
// endpoint (see python-cv/face_analysis.py: result_to_dict). Keeping
// this as a hand-written struct (rather than a shared schema/codegen)
// is a deliberate skeleton-stage simplification — if the two services
// drift, add a contract test that hits /analyze with a fixture image
// and asserts against this struct.
type FaceAnalysis struct {
	Found        bool              `json:"found"`
	FaceShape    string            `json:"face_shape"`
	Measurements *FaceMeasurements `json:"measurements"`
	Landmarks    [][2]float64      `json:"landmarks"`
}

type FaceMeasurements struct {
	FaceWidthPx              float64 `json:"face_width_px"`
	FaceLengthPx             float64 `json:"face_length_px"`
	JawWidthPx               float64 `json:"jaw_width_px"`
	InterpupillaryDistancePx float64 `json:"interpupillary_distance_px"`
	WidthToLengthRatio       float64 `json:"width_to_length_ratio"`
}
