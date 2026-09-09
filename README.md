# Fashion/Eyewear Virtual Try-On — MVP Skeleton

B2B AI infra API for Indian D2C brands. Brand embeds a JS SDK on their site →
calls our Go API → Go API talks to the Python CV service for face/body
analysis and try-on generation.

## Architecture

```
Brand Website
    |
    v
JS SDK (sdk/tryon-widget.js)   <-- captures customer photo ONCE, reuses it
    |
    v
Go API (go-api/)               <-- auth, rate limiting, orchestration, storage
    |
    v
Python CV service (python-cv/) <-- face landmarks, measurements, try-on
    |
    v
Postgres (customers, brands, catalog refs)
Redis   (session cache, stored "photo embedding" for reuse)
MinIO   (S3-compatible object storage for images)
```

## Core design decision (from our discussion)

- Customer photo is captured **once** (on first try-on click), stored securely,
  and reused for every subsequent product on that brand's site.
- Garment-side image is whatever the brand already has in their catalog —
  no upload needed from brand or customer for that.
- Start with a **compositing / AR-style overlay** approach (cheap, fast,
  CPU-only, works for eyewear day one). Photorealistic diffusion-based
  clothing try-on (CatVTON/IDM-VTON style) is a **phase 2** GPU-backed
  upgrade path — same API contract, swap the internals.

## Running locally

```bash
cp .env.example .env
docker compose up --build
```

- Go API: http://localhost:8080
- Python CV service: http://localhost:8000
- MinIO console: http://localhost:9001
- Postgres: localhost:5432

## Folder guide

- `go-api/` — Go backend (net/http + chi router), the only thing brands'
  servers or the JS SDK ever talk to directly.
- `python-cv/` — FastAPI service, internal only (not exposed to brands).
  Does face landmark detection (MediaPipe) now; will host the
  diffusion-based clothing model later.
- `sdk/tryon-widget.js` — the single script tag a brand pastes into their
  product page.
- `docs/` — architecture notes, API contract.
