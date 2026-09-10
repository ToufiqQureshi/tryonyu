# Transcript

Append-only log, numbered, oldest first. Each entry = one meaningful
exchange/decision, kept short (a few lines). Read the LAST 3 ENTRIES ONLY
by default — see the rule in `CLAUDE.md`. Add a new numbered entry after
every real exchange (a decision made, a question answered, work done) —
don't rewrite old entries, don't skip adding one.

---

1. Founder asked what was understood about the project. Explained: SDK/API
   for Indian D2C brands to embed virtual try-on. Eyewear was the
   original plan (cheap, CPU-only, commoditized). Clothing is the real
   unsolved opportunity (needs GPU/diffusion). Differentiation vs
   TryVastra: embed on brand's own PDP, capture customer photo once,
   reuse forever.

2. Founder said: no, clothing first. Reasoning: no one lets the customer
   see themselves actually wearing the product before buying (Amazon/
   Myntra/Flipkart only show it on a model). He's serious, wants it in a
   few months, "without AI" (clarified: without heavy diffusion AI).

3. Explained two paths: (a) diffusion AI (CatVTON/IDM-VTON) — realistic,
   needs GPU; (b) 2D pose-warp overlay — lighter, CPU-ish, lower quality
   ("pasted-on" look). Recommended starting with (b) for speed.

4. Founder asked for a short stack answer for the 2D-warp approach.
   Answer: MediaPipe (pose) + OpenCV TPS warp + optional CP-VTON weights
   + compositing, same Go+Python backend.

5. Founder shared a Flipkart PDP screenshot (a navy kurta) and asked to
   explain concretely how a customer would see themselves wearing THIS
   exact product. Walked through the 7-step pipeline (photo once → pose
   → segmentation → garment extraction → warp → composite → output)
   using that example.

6. Founder said: he has AWS 1-year free tier AND Google Cloud, don't
   worry about GPU — if the product is good, funding will come later.
   Decision: go with the better-quality diffusion approach (IDM-VTON/
   CatVTON) instead of the CPU-warp compromise, using GCP's free GPU
   credit ($300 trial, has GPU quota — AWS free tier is CPU-only, no
   GPU).

7. Founder said "update kar de" — updated `CLAUDE.md` (strategy section
   rewritten: clothing-first is now the plan, GPU budget noted, original
   eyewear-first reasoning kept as historical context) and `ROADMAP.md`
   (phases reordered: clothing = Phase 2/MVP, eyewear = Phase 4).
   Committed, pushed, opened PR #1 (ToufiqQureshi/tryonyu), subscribed to
   its activity.

8. Founder asked to start the clothing pipeline, short-explained what
   would be built, and asked how future sessions would know what was
   done — is there a command. Answered: no command needed, CLAUDE.md is
   auto-read by every new session; proposed a PROGRESS.md work log.
   Then proceeded directly into implementation without waiting for
   explicit go-ahead on the plan — founder later called this out (see
   entry 10): should have paused for confirmation first.

9. Discovered the actual code (`go-api/`, `python-cv/`, `sdk/`, `docs/`)
   was missing from the repo — it had only ever existed inside an
   uploaded `fashion-tryon-mvp.zip` that was later deleted without being
   committed as tracked files. Extracted it from git history and
   restored it as normal tracked files (commit `81430dc`).

10. Built the clothing pipeline scaffolding (commit `fde5920`): real
    CPU-only pose+segmentation (`python-cv/body_pose.py`, MediaPipe),
    an honest not-yet-implemented diffusion interface
    (`python-cv/clothing_generation.py` — raises `NotImplementedError`,
    no GPU in this sandbox), an in-memory async job store
    (`python-cv/jobs.py`), new `/tryon/clothing` + `/tryon/clothing/{id}`
    endpoints, `go-api` async routing + `GET /api/v1/tryon/{jobId}`, and
    SDK polling support. Founder then pushed back: this was done without
    looping him in on the plan first — acknowledged, agreed to confirm
    before big steps going forward. `go build` was never actually run to
    verify the Go code compiles — still unverified.

11. Founder asked for a way for future sessions to get full context
    without re-explaining, worried about token cost of a growing full
    transcript. Created `PROGRESS.md` (short, dated, high-level entries
    — always read) and, in this entry, `TRANSCRIPT.md` (this file — full
    detail, append-only, numbered; `CLAUDE.md` is being updated to say
    "read only the last 3 entries by default" and "append a new entry
    after every real exchange" so any AI agent picks this up correctly
    without reading the whole growing file.

12. Founder marked PR #1 ready for review (out of draft). Verified the
    previously-unverified code: `go build ./...` in `go-api` compiles
    clean, `python3 -m py_compile` passes on all `python-cv/*.py` files.
    Updated the PR description (was stale — still said "docs-only
    change" from the first commit, but the PR now has 23 files/~1900
    lines including all the restored + new code). CodeRabbit review is
    queued (was skipped while draft). Not yet done: an actual `docker
    compose up` end-to-end smoke test.
