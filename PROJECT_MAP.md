# Project Map — read this before touching code in Claude Code

This file exists so a fresh Claude Code session (or a new dev) can orient
in 2 minutes instead of reading every file cold. It lists every file,
what it does, what it uses, and *why* it's built the way it is —
including the decisions we made together before writing any code.

---

## Big picture — why this shape

```
Brand Website
    |
    v
JS SDK (sdk/tryon-widget.js)    <- one script tag, captures customer photo ONCE
    |
    v
Go API (go-api/)                <- the only thing the browser ever talks to
    |
    v
Python CV service (python-cv/)  <- internal only, does the actual face/CV work
    |
    v
Postgres + Redis + S3 (MinIO)   <- storage layer
```

Three decisions drove every file below:

1. **Photo captured once, reused everywhere.** The single biggest UX
   problem with try-on (most customers won't upload a photo per product)
   is solved by asking for it exactly once per customer per brand, then
   reusing the stored face landmarks for every future click.
2. **CPU-only for the eyewear MVP, no paid AI APIs.** Face landmarks via
   MediaPipe run in ~30-80ms on a normal CPU core. Zero GPU cost, zero
   per-call vendor bill — this is what makes the business model viable
   at low price points.
3. **Zero third-party Go dependencies.** This sandbox's network blocks
   `proxy.golang.org`, so instead of fighting that, the Go side is
   written against the standard library only (Go 1.22's built-in
   method-aware router, hand-rolled AWS SigV4 signing instead of an S3
   SDK). This is not a permanent constraint — it's a design choice we
   kept even where it wasn't strictly required, because it makes the
   codebase auditable and immune to upstream dependency breakage. If you
   later want `pgx`, `redis/go-redis`, etc., add them from your own
   machine — `go get` isn't blocked there.

---

## Root files

### `README.md`
The entry point. Architecture diagram, how to run with Docker Compose,
folder guide. Read this first, then this file for per-file detail.

### `docker-compose.yml`
Spins up everything needed to run the whole system locally in one
command: `docker compose up --build`. Services:
- `postgres` — customer/brand/catalog data (schema not written yet — TODO)
- `redis` — cache layer for fast reuse of stored face landmarks (not wired yet — TODO)
- `minio` — S3-compatible object storage, stands in for AWS S3/DigitalOcean
  Spaces/Cloudflare R2 in local dev; `go-api` talks to it via the
  hand-rolled SigV4 client in `internal/storage/s3.go`
- `python-cv` — the face analysis + try-on generation service
- `go-api` — the public-facing API

### `.env.example`
Template for local environment variables (copy to `.env`). Documents
every config value `go-api` and `docker-compose.yml` expect: ports, the
python-cv URL, Postgres connection string, Redis address, and S3/MinIO
credentials.

### `PROJECT_MAP.md`
This file.

---

## `docs/`

### `docs/API_CONTRACT.md`
The four HTTP endpoints `go-api` exposes (`/customers/{id}/photo`,
`/customers/{id}/photo-status`, `/tryon`, `/recommend`) — request/response
shapes and, importantly, *why* each one is shaped that way (e.g. why
try-on and recommendation are separate endpoints, why `category` exists
on `/tryon`). Read this before adding or changing a route.

---

## `go-api/` — the public backend

Written in Go using **only the standard library** (see "why this shape"
above). Brands' browsers and the JS SDK only ever talk to this service —
never directly to `python-cv` or the database.

### `go-api/go.mod`
Module definition. Notice there are **no `require` lines** — that's
deliberate, not incomplete. If you add a real Postgres/Redis client from
your own machine, this file will grow a `require` block then.

### `go-api/main.go`
Entry point. Wires everything together:
- reads env vars (`PORT`, `PYTHON_CV_URL`, `S3_*`)
- constructs the `storage.Client` (S3/MinIO)
- registers routes on Go 1.22's `http.ServeMux` using its built-in
  `"METHOD /path/{param}"` pattern syntax — this is why no router
  library (chi, gorilla, etc.) was needed
- wraps everything in `handlers.WithLogging` and starts the server

### `go-api/internal/handlers/handlers.go`
Shared plumbing for all handlers:
- `Deps` struct — holds everything a handler needs (python-cv URL, the
  S3 client; Postgres/Redis fields are commented in as a TODO map for
  where they'll go)
- `Health` — `GET /health`, used by Docker/uptime checks
- `RequireBrandAPIKey` — auth middleware stub. Currently accepts any
  non-empty `X-API-Key` header so the skeleton runs end-to-end without a
  seeded database. **Not secure — replace with a real Postgres/Redis
  lookup before any real brand uses this.**
- `writeJSON` — tiny helper so every handler returns JSON consistently
- `WithLogging` — one-line-per-request logging middleware

### `go-api/internal/handlers/photo.go`
The one-time photo capture flow — the most important file in the repo,
because it implements the "photo once, reuse everywhere" decision.
- `UploadCustomerPhoto` (`POST /customers/{id}/photo`): reads the
  uploaded photo once into memory, **uploads it to S3** (fully wired,
  via `storage.Client.PutObject`), **forwards it to python-cv's
  `/analyze`** (fully wired, via `analyzePhoto`) to get back face
  landmarks + face shape, and returns that to the caller. **Still TODO:**
  persisting the returned landmarks to Postgres/Redis keyed by
  `customer_id` — the exact `INSERT ... ON CONFLICT` query is written out
  in a comment so it's a copy-paste job once you have a DB driver.
- `analyzePhoto`: builds a `multipart/form-data` request and POSTs it to
  `python-cv`'s `/analyze` endpoint — mirrors exactly what the browser
  SDK does to `go-api`, one hop further in.
- `PhotoStatus` (`GET /customers/{id}/photo-status`): lets the SDK check,
  before showing any UI, whether this customer already has a stored
  photo. Currently hardcoded to `false` — TODO once Postgres/Redis exist.

### `go-api/internal/handlers/tryon.go`
- `TryOn` (`POST /tryon`): the hot path, called on every product-view
  click. Takes `customer_id` + the brand's own `product_image_url` +
  `category`. Comments document the intended routing logic: `eyewear`
  and phase-1 `clothing` go to the cheap CPU landmark-overlay path;
  phase-2 `clothing` (once volume justifies it) would go to an async
  GPU diffusion worker queue. Currently returns a stub URL — wiring the
  real call to `python-cv`'s `/tryon` is the next TODO here.
- `Recommend` (`POST /recommend`): the eyewear-specific wedge feature —
  face-shape based frame recommendations from the **brand's own
  catalog**. Stub response for now; real version joins stored face_shape
  against a `catalog` table.

### `go-api/internal/models/face.go`
Plain Go structs (`FaceAnalysis`, `FaceMeasurements`) that mirror the
exact JSON shape `python-cv`'s `/analyze` endpoint returns. Exists so
`photo.go` can decode that response with `encoding/json` instead of
juggling `map[string]interface{}`. If the two services' JSON shapes ever
drift, this is the file to check first.

### `go-api/internal/storage/s3.go`
A **hand-written AWS SigV4 client** — no `minio-go`, no AWS SDK. Two
methods:
- `PutObject` — uploads photo bytes to `{bucket}/{key}` using
  header-based SigV4 auth. Used by `photo.go` to store the customer's
  one-time photo.
- `PresignGetURL` — generates a time-limited, query-signed GET URL, so
  try-on result images can be served directly from S3/CDN to the
  browser instead of proxying bytes through `go-api`.

  Why hand-rolled instead of a library: this sandbox couldn't reach
  `proxy.golang.org` to fetch `minio-go`, so rather than blocking on
  that, SigV4 was implemented directly against its public spec (it's a
  stable, decade-old algorithm). Bonus: this file works unmodified
  against MinIO, AWS S3, DigitalOcean Spaces, or Cloudflare R2 — you
  only ever change the `S3_ENDPOINT` env var. If you'd rather depend on
  an SDK, this file is a drop-in-replaceable unit; nothing else in the
  codebase needs to change.

### `go-api/internal/storage/s3_test.go`
Two unit tests that don't need a live S3/MinIO server:
- `TestDeriveSigningKey` — checks the 4-step HMAC-SHA256 signing-key
  chain against a known-correct value (independently cross-checked with
  a standalone Python `hmac`/`hashlib` script, not just re-derived with
  the same Go code — see the comment in the test for how to redo that
  check yourself).
- `TestCanonicalizeHeaders` — makes sure signed headers stay sorted and
  lowercase (AWS rejects the whole request with an opaque
  `SignatureDoesNotMatch` if this is even slightly wrong).

Run with `go test ./...` from inside `go-api/`.

### `go-api/Dockerfile`
Two-stage build: compiles the Go binary in a `golang:1.22-alpine`
builder stage, then copies just the binary into a bare `alpine:3.19`
runtime image. No dependency-download step needed since there are no
external Go modules.

---

## `python-cv/` — internal CV/ML service

**Never exposed to brands or the browser** — `go-api` is the only
caller. Kept in Python because the CV/ML ecosystem (MediaPipe, OpenCV,
later PyTorch/ONNX for phase-2 clothing try-on) lives there, and keeping
it out of the Go layer keeps that layer fast and boring.

### `python-cv/requirements.txt`
Pinned dependencies: `fastapi` + `uvicorn` (the HTTP server),
`mediapipe` (face landmark detection — the core CPU-only, zero-cost
model this whole cost strategy depends on), `opencv-python-headless`
(image decode/compositing), `numpy`, `python-multipart` (needed by
FastAPI to parse uploaded files).

### `python-cv/face_analysis.py`
The face landmark + face-shape logic.
- Uses MediaPipe's `FaceMesh` (468 facial landmark points) — CPU-only,
  ~30-80ms per image, no GPU and no per-call vendor cost. This is the
  direct implementation of "avoid expensive external AI APIs."
- `classify_face_shape`: a simple, **explainable ratio heuristic**
  (width-to-length ratio, jaw-to-face-width ratio) rather than a trained
  classifier. Deliberately chosen for v1 because it needs zero labeled
  training data to ship, and the logic is auditable — you can see
  exactly why a face got classified "oval" vs "round." Swap for a
  trained model later only if real usage shows the heuristic isn't
  accurate enough.
- `analyze_face`: the main entry point. Returns a `FaceAnalysisResult`
  with `found`, `face_shape`, `measurements`, and normalized (0–1 range)
  `landmarks` — normalized so they're reusable regardless of the
  original photo's resolution.

### `python-cv/tryon_overlay.py`
Phase-1 eyewear try-on: **landmark-based AR overlay, not a diffusion
model.** This mirrors what every $39/month Shopify eyewear try-on app
actually does under the hood — position a glasses cutout image using
eye landmarks, no GPU required.
- `overlay_glasses`: scales the brand's glasses PNG (transparent
  background) based on interpupillary distance, rotates it to match the
  eye-line angle, and alpha-composites it onto the customer's stored
  photo.
- `_alpha_composite`: the actual pixel-blending math, with bounds
  clipping so an oversized/offset overlay never crashes on out-of-frame
  coordinates.
- This same function signature is meant to be reused for a rough
  clothing "silhouette fit" in phase 1 — full photorealistic clothing
  try-on (CatVTON/IDM-VTON-style diffusion) is an explicitly separate,
  later, GPU-backed phase — see the module docstring for the reasoning
  we worked through on cost/quality tradeoffs.

### `python-cv/main.py`
FastAPI app exposing:
- `GET /health`
- `POST /analyze` — called once per customer by `go-api`'s
  `UploadCustomerPhoto`. Runs `face_analysis.analyze_face` and returns
  the result as JSON (matching `go-api/internal/models/face.go`'s
  struct shape). Returns HTTP 422 with `no_face_detected` if MediaPipe
  can't find a face — `go-api` passes that straight through so the SDK
  can prompt for a clearer photo.
- `POST /tryon` — currently a stub; wiring `tryon_overlay.overlay_glasses`
  in here (for the `eyewear` category) is the next concrete step.

### `python-cv/Dockerfile`
`python:3.11-slim` base plus the system libraries MediaPipe/OpenCV need
at runtime (`libgl1`, `libglib2.0-0` — without these the container
crashes on import with a cryptic `.so` loading error, so they're called
out explicitly rather than left to trial-and-error).

---

## `sdk/` — what the brand actually embeds

### `sdk/tryon-widget.js`
The single `<script>` tag a brand pastes onto a product page. This is
where the "photo once, reuse everywhere" decision becomes a concrete
UX flow:
1. Generates (or reads) a persistent `customer_id` from `localStorage`
   (or uses one the brand passes in, if they have their own logged-in
   customer IDs).
2. Calls `GET /photo-status` — if the customer already has a stored
   photo, it renders a single "Try it on" button with zero friction.
3. If not, it shows a one-time "take a photo" prompt, uploads it via
   `POST /customers/{id}/photo`, and only after that succeeds does the
   "Try it on" button appear — and it never asks again after this.
4. `POST /tryon` is what actually runs on every click after that,
   passing the already-known `customer_id` and the product's own image
   URL (the brand never re-uploads anything per try-on).

This file has zero build step and zero dependencies on purpose — a
brand should be able to paste it in and have it work without a bundler.

---

## What's a stub vs what's real right now

| Piece | Status |
|---|---|
| Go API routing, auth stub, JSON responses | Real, tested, builds clean |
| S3/MinIO photo upload (`PutObject`) | Real, SigV4 math independently verified |
| S3 presigned result URLs (`PresignGetURL`) | Real, not yet called from any handler |
| `go-api` → `python-cv` `/analyze` call | Real, wired in `photo.go` |
| MediaPipe face landmark + face-shape detection | Real |
| Eyewear landmark-overlay try-on math | Real (`tryon_overlay.py`), not yet wired into `python-cv/main.py`'s `/tryon` endpoint |
| Postgres persistence (customer photos, brands, catalog) | **Not started** — needs a driver (`pgx` or `lib/pq`), fetch it from your own machine |
| Redis caching | **Not started** — same reason |
| `/tryon` full wiring in `go-api` | Stub — returns a hardcoded URL |
| Clothing diffusion try-on (phase 2) | Not started — deliberately deferred until eyewear MVP has real usage/revenue data |

---

## Suggested order of work in Claude Code

1. Get `docker compose up --build` running locally, confirm all 5
   containers come up and `/health` responds on both `go-api` and
   `python-cv`.
2. Add a Postgres driver (`go get github.com/jackc/pgx/v5` — works fine
   from your machine) and write the schema for `customers`,
   `customer_photos`, `brands`, `catalog`. The exact query shape for
   `customer_photos` is already sketched in `photo.go`'s comments.
3. Wire that into `PhotoStatus` and the TODO in `UploadCustomerPhoto`.
4. Wire `python-cv/main.py`'s `/tryon` to actually call
   `tryon_overlay.overlay_glasses` for `category == "eyewear"`.
5. Wire `go-api`'s `TryOn` handler to call `python-cv`'s `/tryon` for
   real, and use `storage.Client.PresignGetURL` to hand back a real
   result URL instead of the stub.
6. Only after that loop works end-to-end for eyewear: start phase-2
   clothing (GPU diffusion) research.
