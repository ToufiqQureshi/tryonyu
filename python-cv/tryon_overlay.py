"""
Phase 1 try-on: landmark-based overlay (AR-style compositing).

This is deliberately NOT a diffusion model. For eyewear specifically this
is the industry-standard approach (see: nearly every $39/month Shopify
try-on app) and it's basically free to run — no GPU, sub-second latency.

Approach:
  1. We already have the customer's face landmarks (from face_analysis.py),
     stored once and reused.
  2. We take the brand's product image (glasses PNG with transparent
     background — brands provide this once per SKU, it's a one-time
     catalog prep step, not a per-request cost).
  3. We scale/rotate/position the glasses PNG using the eye landmarks
     (interpupillary distance sets scale, eye-line angle sets rotation)
     and alpha-composite it onto the customer's stored photo.

For clothing, the same function signature holds but "positioning" is
soft-body approximate (shoulders/torso landmarks) rather than pixel-
perfect drape — that's the honest limitation until phase 2 (diffusion)
is warranted by volume/revenue.
"""

from dataclasses import dataclass

import cv2
import numpy as np


@dataclass
class OverlayResult:
    image_bgr: np.ndarray
    method: str = "landmark_overlay"


def overlay_glasses(
    person_image_bgr: np.ndarray,
    glasses_rgba: np.ndarray,
    left_eye_px: tuple,
    right_eye_px: tuple,
) -> OverlayResult:
    """
    person_image_bgr: the customer's stored photo (BGR, no alpha)
    glasses_rgba:      brand's product cutout (BGRA, transparent background)
    left_eye_px/right_eye_px: pixel coords from stored face landmarks
    """
    canvas = person_image_bgr.copy()

    dx = right_eye_px[0] - left_eye_px[0]
    dy = right_eye_px[1] - left_eye_px[1]
    eye_distance = float(np.hypot(dx, dy))
    angle_deg = float(np.degrees(np.arctan2(dy, dx)))

    # Glasses are usually ~2.2x wider than the interpupillary distance —
    # tune this per-catalog if brands provide their own frame width in mm.
    target_width = eye_distance * 2.2
    scale = target_width / glasses_rgba.shape[1]
    new_w = max(1, int(glasses_rgba.shape[1] * scale))
    new_h = max(1, int(glasses_rgba.shape[0] * scale))
    resized = cv2.resize(glasses_rgba, (new_w, new_h), interpolation=cv2.INTER_AREA)

    center = (new_w // 2, new_h // 2)
    rot_mat = cv2.getRotationMatrix2D(center, -angle_deg, 1.0)
    rotated = cv2.warpAffine(
        resized, rot_mat, (new_w, new_h),
        flags=cv2.INTER_AREA, borderMode=cv2.BORDER_CONSTANT, borderValue=(0, 0, 0, 0),
    )

    mid_x = int((left_eye_px[0] + right_eye_px[0]) / 2)
    mid_y = int((left_eye_px[1] + right_eye_px[1]) / 2)
    top_left_x = mid_x - new_w // 2
    top_left_y = mid_y - new_h // 2

    _alpha_composite(canvas, rotated, top_left_x, top_left_y)
    return OverlayResult(image_bgr=canvas)


def _alpha_composite(base_bgr: np.ndarray, overlay_bgra: np.ndarray, x: int, y: int) -> None:
    """In-place alpha composite of overlay_bgra onto base_bgr at (x, y),
    clipped to base bounds."""
    h, w = overlay_bgra.shape[:2]
    bh, bw = base_bgr.shape[:2]

    x0, y0 = max(x, 0), max(y, 0)
    x1, y1 = min(x + w, bw), min(y + h, bh)
    if x0 >= x1 or y0 >= y1:
        return

    ox0, oy0 = x0 - x, y0 - y
    ox1, oy1 = ox0 + (x1 - x0), oy0 + (y1 - y0)

    overlay_crop = overlay_bgra[oy0:oy1, ox0:ox1]
    alpha = overlay_crop[:, :, 3:4].astype(np.float32) / 255.0
    base_region = base_bgr[y0:y1, x0:x1].astype(np.float32)
    blended = overlay_crop[:, :, :3].astype(np.float32) * alpha + base_region * (1 - alpha)
    base_bgr[y0:y1, x0:x1] = blended.astype(np.uint8)
