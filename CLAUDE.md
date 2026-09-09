# CLAUDE.md — read this FIRST, before touching any code

This file is for Claude Code (or any dev) picking up this project fresh.
It has zero memory of the conversation where this project was designed —
this file replaces that. Read this, then `PROJECT_MAP.md` for per-file
detail, then `ROADMAP.md` for what to build next, in order.

---

## Who is building this, and why

Founder: Indian, building solo/early-stage. No existing engineering team.
Domain already bought: **tryonyu.com**.

### The original idea (where we started)

Big Indian platforms (Lenskart, Myntra) can afford to build AI features
like virtual try-on and face-shape recommendations in-house — GPU
infra, ML teams, months of engineering. Small and mid-size Indian D2C
brands have good products and their own websites, but cannot afford to
build this themselves. The idea: sell this as an API/SDK — brands
integrate a script tag, we run the AI infra, they get the feature
without building it.

### What research actually found (don't skip this — it changed the plan)

We did NOT just build what was asked blindly. We researched the market
first and it materially changed direction:

1. **Eyewear virtual try-on is already commoditized.** Multiple Shopify
   apps (Camweara $39-200/mo, TryOnMe, GenLook, Antla) already do
   face-landmark-based glasses overlay. It's a solved, cheap, low-moat
   problem — CPU-only, no GPU needed, works via MediaPipe FaceMesh.
   **Conclusion: eyewear alone is not a strong standalone business**,
   but it's a legitimate, fast-to-build MVP/wedge because it's
   technically easy and proves the infra works end-to-end.

2. **Clothing virtual try-on is NOT solved.** This is the real
   opportunity. It requires diffusion models (CatVTON, IDM-VTON) that
   need GPU (A100-class), take 15-60 seconds per generation, and cost
   ₹0.5-2 per image at real-time scale. Nobody has cracked "good,
   fast, cheap" simultaneously yet — this is where actual differentiation
   is possible, not eyewear.

3. **India-specific competitors exist but split into two very different
   product types** — this distinction matters a lot, don't confuse them:
   - **Seller-side catalog generation tools** (CatalogX, StitchMagic):
     brands upload a garment photo, get a photorealistic *catalog/model
     photo* back, to replace studio photoshoots. The END CUSTOMER never
     touches these. Not our competitor — different product.
   - **Customer-facing try-on** (TryVastra.ai): the shopper uploads
     their own photo and sees themselves in the outfit before buying.
     **This is our actual competitive category** — and it's much less
     crowded than the seller-side catalog tools.

4. **TryVastra's actual flaw, which is our differentiation**: it's a
   standalone tool (separate website, manual multi-step upload of BOTH
   a person photo and a product photo every single time, "buy credits"
   system). No real brand (Snitch/Bewakoof-scale) will ever integrate
   something that clunky into their checkout flow. The founder (in this
   conversation) identified the fix himself: **the flow must be embedded
   directly on the brand's own product page**, and **the customer's
   photo must be captured exactly once and reused for every product**
   — never re-uploaded. This is the single most important product
   decision in this repo — see `sdk/tryon-widget.js` and
   `PROJECT_MAP.md`'s "why this shape" section for how it's implemented.

5. **Real market pain point (why brands would actually pay):** Indian
   fashion e-commerce returns run 25-40%, and 53-70% of fashion returns
   are specifically due to wrong size/fit — costing the industry an
   estimated ₹2 lakh crore/year. Myntra is already investing heavily in
   AI (size recommendation covering 85% of catalog, ~20% conversion
   lift claimed) — proof big players see this as real ROI, not a gimmick.
   **The pitch to brands should be "reduce returns," not "cool AI
   feature."**

### The actual strategy (in priority order)

1. **Ship eyewear first** — technically simple (CPU-only landmark
   overlay, no GPU, no diffusion model needed), fast to build, gives
   real usage data and a working end-to-end pipeline (SDK → API →
   CV service → storage) to sell to a first few brands.
2. **Design the whole system so eyewear and clothing share the same API
   contract** — `category` field on `/tryon` routes to different
   backends (`landmark_overlay` for eyewear now, a GPU diffusion queue
   for clothing later) without brands' integration code ever changing.
   See `docs/API_CONTRACT.md`.
3. **The "photo once, reuse everywhere" SDK UX is the core moat**, more
   than the CV model itself — this is what actually differentiates from
   TryVastra and other standalone try-on tools.
4. **Move into clothing try-on (phase 2) only once eyewear has real
   brand usage** — clothing needs GPU spend, so it should be justified
   by revenue/volume, not built speculatively first.
5. **Positioning for India**: INR pricing, WhatsApp-first onboarding/
   support, ethnic wear as an underserved niche (most global try-on
   tools are built for Western clothing), and "cut your return rate" as
   the sales pitch to brand founders — not "we have AI."

---

## Engineering decisions already made — and why (don't relitigate these)

- **Go for the public API, Python for CV/ML** — matches the original
  brief; Go is fast/boring for the request-routing layer, Python has
  the ML ecosystem (MediaPipe now, PyTorch/ONNX later for clothing).
- **No GPU, no paid AI APIs for the eyewear MVP.** MediaPipe FaceMesh
  runs CPU-only in ~30-80ms. This is what makes the unit economics work
  at a price point small Indian brands can actually afford. GPU is
  deliberately deferred to phase 2 (clothing), and only once volume
  justifies the cost.
- **Zero third-party Go dependencies right now.** This was originally
  forced by a sandbox restriction (`proxy.golang.org` was blocked in the
  environment this was built in) but was kept deliberately even where
  not strictly required: `internal/storage/s3.go` hand-rolls AWS SigV4
  signing instead of using `minio-go`, and `main.go` uses Go 1.22's
  built-in `http.ServeMux` pattern routing instead of `chi`/`gorilla`.
  **This constraint does NOT apply in Claude Code on your own machine
  — `go get` will work fine there.** Feel free to add `pgx`,
  `go-redis`, etc. Just don't rip out the S3 client without reason —
  it was verified correct (unit-tested against an independently
  computed SigV4 test vector) and it's provider-agnostic (works
  unmodified against MinIO, AWS S3, DigitalOcean Spaces, Cloudflare R2).
- **Face-shape classification is a hand-written ratio heuristic, not a
  trained model** (see `python-cv/face_analysis.py`). Deliberate: zero
  training data needed to ship v1, fully explainable. Only replace with
  a trained classifier if real-world accuracy turns out to be bad.
- **Recommendation and try-on are separate API endpoints** (`/recommend`
  vs `/tryon`) so a brand can use face-shape recommendation without
  try-on (e.g., a quiz-style "find your frame" landing page) or vice
  versa.

---

## What is real code vs stub right now

See the table in `PROJECT_MAP.md` under "What's a stub vs what's real
right now" — read it before assuming any endpoint fully works.
Short version: Go API structure + S3 upload + calling python-cv's
`/analyze` are real and tested. Postgres/Redis persistence and the
actual `/tryon` generation call are still stubs with TODOs and exact
target SQL/logic written in code comments.

---

## Read next

1. `PROJECT_MAP.md` — every file, what it does, why it's shaped that way.
2. `ROADMAP.md` — what's left to build, in the order to build it.
3. `docs/API_CONTRACT.md` — the API shape and the reasoning behind it.
