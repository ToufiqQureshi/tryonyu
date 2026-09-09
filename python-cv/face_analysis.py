"""
Face landmark detection + face-shape classification.

Uses MediaPipe FaceMesh — CPU-only, no GPU needed, runs in ~30-80ms per
image on a normal server core. This is the "avoid expensive external AI
APIs" decision from our research: no per-call cost, no vendor, fully
under our control.

Face-shape classification here is a simple, explainable ratio heuristic
(width/length, jaw width, cheekbone width) rather than a trained
classifier. It's good enough for a recommendation feature and has zero
training-data requirement to ship v1. Swap in a trained model later if
the heuristic's accuracy isn't good enough in practice.
"""

from dataclasses import dataclass, asdict
from typing import Optional

import cv2
import mediapipe as mp
import numpy as np

mp_face_mesh = mp.solutions.face_mesh

# A handful of MediaPipe FaceMesh landmark indices we actually need.
# (Full mesh has 468 points; we only care about a few for measurements.)
LEFT_CHEEK = 234
RIGHT_CHEEK = 454
LEFT_JAW = 172
RIGHT_JAW = 397
CHIN = 152
FOREHEAD = 10
LEFT_EYE_OUTER = 33
RIGHT_EYE_OUTER = 263


@dataclass
class FaceMeasurements:
    face_width_px: float
    face_length_px: float
    jaw_width_px: float
    interpupillary_distance_px: float
    width_to_length_ratio: float


@dataclass
class FaceAnalysisResult:
    found: bool
    face_shape: Optional[str] = None
    measurements: Optional[FaceMeasurements] = None
    landmarks: Optional[list] = None  # normalized (x, y) pairs, for reuse


def _dist(a, b) -> float:
    return float(np.hypot(a[0] - b[0], a[1] - b[1]))


def classify_face_shape(m: FaceMeasurements) -> str:
    """Simple, explainable heuristic — tune thresholds against real
    labeled photos before trusting this in production."""
    ratio = m.width_to_length_ratio
    jaw_to_face = m.jaw_width_px / m.face_width_px if m.face_width_px else 0

    if ratio > 0.95:
        return "round" if jaw_to_face > 0.85 else "square"
    if ratio < 0.75:
        return "oblong"
    if jaw_to_face < 0.75:
        return "heart"
    return "oval"


def analyze_face(image_bgr: np.ndarray) -> FaceAnalysisResult:
    h, w = image_bgr.shape[:2]

    with mp_face_mesh.FaceMesh(
        static_image_mode=True,
        max_num_faces=1,
        refine_landmarks=True,
        min_detection_confidence=0.5,
    ) as face_mesh:
        results = face_mesh.process(cv2.cvtColor(image_bgr, cv2.COLOR_BGR2RGB))

        if not results.multi_face_landmarks:
            return FaceAnalysisResult(found=False)

        lm = results.multi_face_landmarks[0].landmark
        pts = {i: (lm[i].x * w, lm[i].y * h) for i in range(len(lm))}

        measurements = FaceMeasurements(
            face_width_px=_dist(pts[LEFT_CHEEK], pts[RIGHT_CHEEK]),
            face_length_px=_dist(pts[FOREHEAD], pts[CHIN]),
            jaw_width_px=_dist(pts[LEFT_JAW], pts[RIGHT_JAW]),
            interpupillary_distance_px=_dist(pts[LEFT_EYE_OUTER], pts[RIGHT_EYE_OUTER]),
            width_to_length_ratio=0.0,
        )
        if measurements.face_length_px:
            measurements.width_to_length_ratio = (
                measurements.face_width_px / measurements.face_length_px
            )

        shape = classify_face_shape(measurements)

        # Normalized landmarks (0-1 range) are what we persist — reusable
        # across sessions without keeping the raw photo hot anywhere.
        normalized = [(round(lm[i].x, 5), round(lm[i].y, 5)) for i in range(len(lm))]

        return FaceAnalysisResult(
            found=True,
            face_shape=shape,
            measurements=measurements,
            landmarks=normalized,
        )


def result_to_dict(result: FaceAnalysisResult) -> dict:
    d = asdict(result)
    return d
