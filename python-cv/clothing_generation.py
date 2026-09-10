"""
Clothing try-on generation — the diffusion step (CatVTON / IDM-VTON).

THIS FILE IS THE ONE HONEST GAP IN THE CLOTHING PIPELINE RIGHT NOW.
Everything upstream (body_pose.py's pose + segmentation) is real, runs
CPU-only, and is unit-testable today. This function is a defined
interface with NO model wired in yet, because running CatVTON/IDM-VTON
needs GPU weights + inference code that don't belong in this repo until
someone has actually benchmarked the two on a GPU box (see ROADMAP.md
Phase 2) and picked one.

Wiring a real model here means, concretely:
  1. Stand up a GPU worker (GCP T4 from the free-trial credit — see
     CLAUDE.md) with CatVTON or IDM-VTON's inference code + weights.
  2. Expose that worker over HTTP or a job queue (Redis Streams is
     suggested in ROADMAP.md).
  3. Replace the `raise NotImplementedError` below with the actual call
     — the function signature already matches what both CatVTON and
     IDM-VTON expect (person image, person mask, pose keypoints, garment
     image) so the call site (jobs.py) doesn't need to change.

Callers should treat NotImplementedError from this function as an
expected "GPU worker not connected yet" state, not a bug — jobs.py
catches it and reports a clear job status rather than crashing.
"""

from dataclasses import dataclass

import numpy as np


@dataclass
class GenerationResult:
    image_bgr: np.ndarray
    method: str  # "catvton" | "idm_vton"


def generate_clothing_tryon(
    person_image_bgr: np.ndarray,
    person_mask: np.ndarray,
    pose_landmarks: list,
    garment_image_bgr: np.ndarray,
) -> GenerationResult:
    """
    person_image_bgr:  customer's stored photo (BGR)
    person_mask:       segmentation mask from body_pose.analyze_body,
                        upsampled to person_image_bgr's resolution
    pose_landmarks:    normalized (x, y, visibility) triples from
                        body_pose.analyze_body — used by the diffusion
                        model's pose-conditioning input
    garment_image_bgr: the brand's product image for this SKU (fetched
                        from ProductImageURL, never re-uploaded by hand)

    Returns the generated try-on image. Raises NotImplementedError until
    a GPU worker is wired in (see module docstring).
    """
    raise NotImplementedError(
        "No diffusion backend wired in yet. This needs a GPU worker "
        "running CatVTON or IDM-VTON (see ROADMAP.md Phase 2) — the "
        "preprocessing this function expects (person_mask, pose_landmarks) "
        "is already produced by body_pose.analyze_body, so only the "
        "actual model call needs to be added here."
    )
