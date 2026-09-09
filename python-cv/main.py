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
from fastapi import FastAPI, File, UploadFile, HTTPException
from fastapi.responses import JSONResponse

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
    Stub for the try-on generation endpoint. Real version:
      - input: stored landmarks (not a re-uploaded photo) + product image
        URL + category
      - category == eyewear -> tryon_overlay.overlay_glasses(...)
      - category == clothing (phase 2) -> enqueue to GPU worker running
        CatVTON/IDM-VTON, return a job id for async polling since that
        path takes 15-60s, not a synchronous response
    """
    return {"status": "not_implemented", "note": "wire tryon_overlay.py here for eyewear MVP"}


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=int(os.getenv("PORT", "8000")))
