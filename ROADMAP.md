# Roadmap

Read `CLAUDE.md` first if you haven't — it has the business context that
explains *why* this order was chosen. This file is the concrete task
list, in the order it should be tackled.

Status legend: `[ ]` not started · `[~]` stub exists, needs real wiring · `[x]` done

---

## Phase 0 — Get the skeleton running (do this first, don't skip)

- [ ] `cp .env.example .env`, then `docker compose up --build` from repo root
- [ ] Confirm all 5 containers are healthy: `postgres`, `redis`, `minio`,
      `python-cv`, `go-api`
- [ ] `curl http://localhost:8080/health` → `{"status":"ok"}`
- [ ] `curl http://localhost:8000/health` → `{"status":"ok"}`
- [ ] Open MinIO console (`http://localhost:9001`, creds in
      `docker-compose.yml`) and manually create the `tryon-photos` bucket
      — nothing auto-creates it yet (small TODO: add bucket-creation
      on startup, either in `go-api`'s `main.go` or a compose init step)

## Phase 1 — Persistence (Postgres + Redis)

This is the biggest real gap right now — everything else routes through
it. The exact schema below is a starting point, not gospel — adjust as
you build.

- [ ] `go get github.com/jackc/pgx/v5` (from your own machine — this
      sandbox's proxy block doesn't apply to you)
- [ ] Write the schema (suggested starting tables):
  ```sql
  CREATE TABLE brands (
    id TEXT PRIMARY KEY,
    api_key_hash TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now()
  );

  CREATE TABLE customer_photos (
    customer_id TEXT PRIMARY KEY,
    brand_id TEXT REFERENCES brands(id),
    s3_key TEXT NOT NULL,
    face_shape TEXT,
    landmarks JSONB,
    created_at TIMESTAMPTZ DEFAULT now()
  );

  CREATE TABLE catalog (
    sku TEXT PRIMARY KEY,
    brand_id TEXT REFERENCES brands(id),
    category TEXT NOT NULL,          -- 'eyewear' | 'clothing_top' | ...
    image_url TEXT NOT NULL,
    suited_face_shapes TEXT[]        -- for /recommend, eyewear only
  );
  ```
- [ ] Wire `RequireBrandAPIKey` (`internal/handlers/handlers.go`) to do a
      real lookup against `brands.api_key_hash` instead of accepting any
      non-empty header
- [ ] Wire the TODO in `UploadCustomerPhoto` (`internal/handlers/photo.go`)
      to `INSERT ... ON CONFLICT` into `customer_photos` — the exact
      query shape is already sketched in that file's comments
- [ ] Wire `PhotoStatus` to check Redis first, fall back to Postgres,
      cache on read
- [ ] Add a Redis client (`go-redis/redis/v9` or similar) for the cache
      layer described above

## Phase 2 — Clothing try-on MVP (REVISED — this is now the priority, not eyewear)

Per `CLAUDE.md`: the founder has GPU budget (Google Cloud free-trial
credit with GPU quota, plus AWS free tier for the non-GPU pieces) and
has deliberately chosen to go straight at clothing — the harder,
uncrowded, actually-differentiated problem — instead of the eyewear
wedge. Eyewear moves to Phase 4 as a cheap add-on later.

- [ ] Spin up a Google Cloud GPU instance (T4, from the free-trial
      credit) as the dev/eval box for the diffusion service
- [ ] Benchmark CatVTON vs IDM-VTON yourself on that GPU box (research
      found CatVTON: ~11s on A100, <8GB VRAM, CC BY-NC-SA license needs
      a commercial license check; IDM-VTON: ~17s on A100, better texture
      fidelity) — pick based on real output quality on Indian ethnic
      wear specifically (sarees, kurtas, short kurtas, lehengas), not
      just benchmarks from their papers
- [ ] In `python-cv/`, add the clothing pipeline: MediaPipe pose +
      segmentation on the customer's stored photo → feed pose +
      garment product image into the chosen diffusion model → return
      generated image
- [ ] Since clothing generation is slow (15-60s), the `/tryon` contract
      for `category: "clothing_*"` needs to be async: return a job ID
      immediately, add a `/tryon/{jobId}` polling endpoint or a
      webhook, and update `sdk/tryon-widget.js` to poll/wait instead of
      expecting a synchronous response
- [ ] Add a GPU worker queue (this is genuinely new infrastructure, not
      a small addition — budget real time for it: job queue like Redis
      Streams or a proper queue service, worker process, GPU instance
      provisioning/autoscaling)
- [ ] Wire `go-api`'s `TryOn` handler (`internal/handlers/tryon.go`) to:
      1. look up the customer's stored photo (Redis/Postgres, not
         re-uploaded — this is the core "photo once" moat)
      2. enqueue the job with `python-cv`'s clothing pipeline
      3. store the result image via `storage.Client.PutObject`
      4. return a real URL via `storage.Client.PresignGetURL` once the
         job completes, replacing the current hardcoded stub
- [ ] Cost-model this before shipping: research found generation costs
      of ₹0.5-2/image — model out what that means at your actual
      expected volume before pricing plans to brands
- [ ] Ethnic-wear specific handling: draping/pleats/pallu for sarees
      behave very differently from fitted Western clothing — this may
      need fine-tuning or pre/post-processing beyond stock CatVTON/
      IDM-VTON, budget time to actually test on real sarees/lehengas/
      short kurtas (e.g. the Flipkart-style product page case)
- [ ] End-to-end manual test: use `sdk/tryon-widget.js` against a real
      static HTML page styled like a PDP (see reference screenshot —
      a Flipkart-style kurta listing), upload a real photo, get back a
      real generated try-on image

## Phase 3 — Make it demo-able / sellable to a first brand

- [ ] Brand onboarding flow: how does a brand get an API key and upload
      their catalog (garment images)? Doesn't need to be fully
      self-serve yet — a manual/admin script is fine for the first few
      brands.
- [ ] Basic rate limiting per brand (protects against runaway GPU
      costs — this matters much more for clothing than it did for
      CPU-only eyewear overlay)
- [ ] Deploy `go-api` + `python-cv` somewhere real (see "Deployment"
      below) instead of only local Docker Compose
- [ ] Get 1-3 real Indian D2C clothing brands to actually try the
      embedded widget on a real product page — this is the validation
      step, don't skip straight to phase 4 without it

## Phase 4 — Eyewear try-on (cheap add-on, build once clothing has real usage)

Eyewear is now the *later* phase — it's the commoditized, low-moat
problem, kept in scope only because it's cheap to bolt on (CPU-only,
MediaPipe FaceMesh, no GPU) and can round out the product once clothing
has proven the core moat. See `CLAUDE.md` for the full reasoning on why
this was originally phase 1 and got reordered.

- [ ] In `python-cv/main.py`, wire the `/tryon` stub to actually call
      `tryon_overlay.overlay_glasses()` for `category == "eyewear"`:
      needs the customer's stored landmarks (passed in from `go-api`,
      not re-uploaded) and the brand's glasses PNG (transparent bg)
- [ ] Decide how brands supply the transparent-background glasses cutout
      per SKU — likely a one-time catalog onboarding step, not a live
      upload. Document the expected image spec (min resolution,
      background requirements) somewhere brand-facing.
- [ ] Wire `Recommend` (`internal/handlers/tryon.go`) to join stored
      `face_shape` against `catalog.suited_face_shapes`
- [ ] End-to-end manual test: use `sdk/tryon-widget.js` against a real
      static HTML page, upload a real photo, get a real try-on result
      back

## Ongoing / cross-cutting (pick up whenever relevant)

- [ ] Tests: `go-api` currently only has unit tests for
      `internal/storage`. Add handler-level tests once Postgres/Redis
      are wired (use a test DB / testcontainers, not the real dev DB)
- [ ] Observability: structured logging, and at minimum a dashboard of
      try-on success rate / latency / cost-per-call once this has real
      traffic
- [ ] Security pass before any real brand goes live: photo storage
      encryption at rest, a data-retention/deletion policy for customer
      photos (this is biometric-adjacent data — check what India's
      DPDP Act requires here specifically), and a real secrets
      management approach (`.env` files are fine for local dev only)

## Deployment (when ready for phase 3)

Not decided yet — options to evaluate when you get here: Railway/
Render/Fly.io for `go-api` + `python-cv` (simple, fast to set up) vs.
a cloud provider directly (AWS/GCP) if you need more control over GPU
instances for phase 4. Don't over-decide this now — phase 1-2 can run
anywhere Docker runs.
