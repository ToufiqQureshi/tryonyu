"""
Internal-only CV service. Never exposed directly to brands or the browser
SDK — the Go API is the only caller. Kept separate from Go so the CV/ML
stack (mediapipe, opencv, later torch/onnx) stays in its natural
Python environment without dragging that dependency weight into the API
layer that needs to be fast and boring.
"""

import io
import os

import cv2
import numpy as np
import requests
from fastapi import BackgroundTasks, FastAPI, File, HTTPException, UploadFile
from fastapi.responses import JSONResponse
from pydantic import BaseModel

import jobs
from body_pose import analyze_body
from body_pose import result_to_dict as body_result_to_dict
from clothing_generation import generate_clothing_tryon
from face_analysis import analyze_face, result_to_dict

app = FastAPI(title="tryon-cv-service")


def _read_image(upload_bytes: bytes) -> np.ndarray:
    arr = np.frombuffer(upload_bytes, dtype=np.uint8)
    img = cv2.imdecode(arr, cv2.IMREAD_COLOR)
    if img is None:
        raise HTTPException(status_code=400, detail="could not decode image")
    return img


@app.get("/health")
def health():
    return {"status": "ok"}


@app.post("/analyze")
async def analyze(photo: UploadFile = File(...)):
    """
    Called once per customer (from go-api's UploadCustomerPhoto).
    Returns face-shape + normalized landmarks to be persisted — the
    Go API stores this, so we never need to re-run this per try-on call.
    """
    contents = await photo.read()
    image = _read_image(contents)
    result = analyze_face(image)

    if not result.found:
        return JSONResponse(status_code=422, content={
            "found": False,
            "error": "no_face_detected",
            "message": "Ask the customer for a clearer, front-facing photo.",
        })

    return result_to_dict(result)


@app.post("/tryon")
async def tryon():
    """
    Stub for the eyewear (landmark_overlay) try-on generation endpoint.
    Eyewear is now Phase 4 (see ROADMAP.md) — clothing is the current
    priority, wired below via /tryon/clothing.
    """
    return {"status": "not_implemented", "note": "wire tryon_overlay.py here for the eyewear phase"}


@app.post("/body/analyze")
async def body_analyze(photo: UploadFile = File(...)):
    """
    Called once per customer (mirrors /analyze for faces). Returns pose
    landmarks + a coarse segmentation mask to be persisted by go-api —
    every future clothing /tryon call reuses this instead of recomputing
    it from the raw photo.
    """
    contents = await photo.read()
    image = _read_image(contents)
    result = analyze_body(image)

    if not result.found:
        return JSONResponse(status_code=422, content={
            "found": False,
            "error": "no_body_detected",
            "message": "Ask the customer for a clearer, full/half-body photo.",
        })

    return body_result_to_dict(result)


class ClothingTryOnRequest(BaseModel):
    person_photo_url: str  # presigned S3 URL for the customer's stored photo
    product_image_url: str  # brand's own catalog image, fetched not re-uploaded


def _fetch_image(url: str) -> np.ndarray:
    resp = requests.get(url, timeout=15)
    resp.raise_for_status()
    return _read_image(resp.content)


def _run_clothing_generation(job_id: str, req: ClothingTryOnRequest) -> None:
    """Background task: runs on FastAPI's threadpool, not the request
    thread, since generation is expected to take 15-60s once a real
    diffusion backend is wired in (see clothing_generation.py)."""
    try:
        person_image = _fetch_image(req.person_photo_url)
        garment_image = _fetch_image(req.product_image_url)

        body = analyze_body(person_image)
        if not body.found:
            jobs.store.mark_failed(job_id, "no_body_detected in stored photo")
            return

        h, w = person_image.shape[:2]
        mask = np.array(body.segmentation_mask, dtype=np.float32)
        mask = cv2.resize(mask, (w, h), interpolation=cv2.INTER_LINEAR)

        result = generate_clothing_tryon(
            person_image_bgr=person_image,
            person_mask=mask,
            pose_landmarks=body.landmarks,
            garment_image_bgr=garment_image,
        )
        # TODO once a real backend is wired: upload result.image_bgr
        # somewhere fetchable (S3 via go-api, or python-cv gets its own
        # S3 client) and call jobs.store.mark_done(job_id, url).
        jobs.store.mark_done(job_id, "not_yet_persisted")
    except NotImplementedError as e:
        jobs.store.mark_failed(job_id, str(e))
    except Exception as e:  # fetch failures, decode failures, etc.
        jobs.store.mark_failed(job_id, f"generation failed: {e}")


@app.post("/tryon/clothing")
async def tryon_clothing(req: ClothingTryOnRequest, background_tasks: BackgroundTasks):
    """
    Async entrypoint for clothing try-on (Phase 2 — see ROADMAP.md).
    Returns a job id immediately; poll GET /tryon/clothing/{job_id} for
    the result. Currently every job ends up "failed" with a clear
    "GPU worker not wired in yet" message, because clothing_generation.py
    has no diffusion backend connected yet — this endpoint's job/poll
    contract is real and testable end-to-end regardless.
    """
    job = jobs.store.create()
    background_tasks.add_task(_run_clothing_generation, job.id, req)
    return {"job_id": job.id, "status": job.status}


@app.get("/tryon/clothing/{job_id}")
async def tryon_clothing_status(job_id: str):
    job = jobs.store.get(job_id)
    if job is None:
        raise HTTPException(status_code=404, detail="job not found")
    return {"job_id": job.id, "status": job.status, "result_url": job.result_url, "error": job.error}


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=int(os.getenv("PORT", "8000")))
