"""
Body pose + person segmentation for clothing try-on.

Uses MediaPipe Pose + SelfieSegmentation — CPU-only, same "no GPU, no paid
AI API" philosophy as face_analysis.py. This step is NOT the diffusion
generation itself: it's the cheap preprocessing every clothing try-on
needs regardless of which generation backend runs (CatVTON, IDM-VTON, or
a future swap) — shoulder/hip/limb landmarks for garment alignment, and a
person/background mask so the generator only touches the body region.

Like face_analysis.py, this runs once per stored customer photo (not per
try-on call) — the landmarks + mask get persisted and reused, matching the
"photo once, reuse everywhere" decision in CLAUDE.md.
"""

from dataclasses import dataclass, asdict
from typing import Optional

import cv2
import mediapipe as mp
import numpy as np

mp_pose = mp.solutions.pose
mp_selfie_segmentation = mp.solutions.selfie_segmentation

# Subset of MediaPipe Pose's 33 landmarks relevant to garment alignment.
LEFT_SHOULDER = 11
RIGHT_SHOULDER = 12
LEFT_HIP = 23
RIGHT_HIP = 24
LEFT_ELBOW = 13
RIGHT_ELBOW = 14
LEFT_WRIST = 15
RIGHT_WRIST = 16


@dataclass
class BodyMeasurements:
    shoulder_width_px: float
    torso_length_px: float  # shoulder midpoint to hip midpoint


@dataclass
class BodyAnalysisResult:
    found: bool
    measurements: Optional[BodyMeasurements] = None
    landmarks: Optional[list] = None  # normalized (x, y, visibility) triples
    segmentation_mask: Optional[list] = None  # normalized 0-1 mask, downsampled


def _dist(a, b) -> float:
    return float(np.hypot(a[0] - b[0], a[1] - b[1]))


def _midpoint(a, b) -> tuple:
    return ((a[0] + b[0]) / 2, (a[1] + b[1]) / 2)


def analyze_body(image_bgr: np.ndarray, mask_downsample: int = 64) -> BodyAnalysisResult:
    """
    Returns pose landmarks (for garment placement/warping) and a coarse
    person segmentation mask (for compositing — keeps the generator/
    compositor from bleeding onto the background).

    mask_downsample: mask is stored at this max dimension to keep the
    persisted payload small; the generation step re-upsamples as needed.
    """
    h, w = image_bgr.shape[:2]
    rgb = cv2.cvtColor(image_bgr, cv2.COLOR_BGR2RGB)

    with mp_pose.Pose(static_image_mode=True, min_detection_confidence=0.5) as pose:
        pose_results = pose.process(rgb)

    if not pose_results.pose_landmarks:
        return BodyAnalysisResult(found=False)

    lm = pose_results.pose_landmarks.landmark
    pts = {i: (lm[i].x * w, lm[i].y * h) for i in range(len(lm))}

    shoulder_mid = _midpoint(pts[LEFT_SHOULDER], pts[RIGHT_SHOULDER])
    hip_mid = _midpoint(pts[LEFT_HIP], pts[RIGHT_HIP])

    measurements = BodyMeasurements(
        shoulder_width_px=_dist(pts[LEFT_SHOULDER], pts[RIGHT_SHOULDER]),
        torso_length_px=_dist(shoulder_mid, hip_mid),
    )

    normalized_landmarks = [
        (round(p.x, 5), round(p.y, 5), round(p.visibility, 3)) for p in lm
    ]

    with mp_selfie_segmentation.SelfieSegmentation(model_selection=1) as segmenter:
        seg_results = segmenter.process(rgb)
    mask = seg_results.segmentation_mask  # float32, values 0-1, same h/w as input

    scale = mask_downsample / max(mask.shape)
    small_mask = cv2.resize(
        mask, (max(1, int(mask.shape[1] * scale)), max(1, int(mask.shape[0] * scale))),
        interpolation=cv2.INTER_AREA,
    )
    mask_list = np.round(small_mask, 3).tolist()

    return BodyAnalysisResult(
        found=True,
        measurements=measurements,
        landmarks=normalized_landmarks,
        segmentation_mask=mask_list,
    )


def result_to_dict(result: BodyAnalysisResult) -> dict:
    return asdict(result)
