# Progress Log

Read this AFTER `CLAUDE.md` and BEFORE `ROADMAP.md`. `CLAUDE.md` is the
*why* (business context, decisions), `ROADMAP.md` is the *what's left*.
This file is the *what's actually been done so far and why*, kept short
so a new session doesn't have to re-derive it from git log. Add a new
dated entry each time real work lands — don't rewrite old entries.

---

## 2026-09-09 — Strategy pivot: clothing try-on first (not eyewear)

- Original plan (see `CLAUDE.md`, kept for context) was eyewear-first
  because it's cheap/CPU-only and clothing needs GPU. Founder has GPU
  budget now (Google Cloud free-trial credit + AWS free tier) and chose
  to build clothing — the harder, uncrowded, actually-differentiated
  problem — first instead.
- Updated `CLAUDE.md` (strategy section) and `ROADMAP.md` (phase order:
  clothing is now Phase 2/MVP, eyewear moved to Phase 4).
- PR: https://github.com/ToufiqQureshi/tryonyu/pull/1

## 2026-09-09 — Restored missing code from repo

- Repo on GitHub had only doc files (`CLAUDE.md`, `PROJECT_MAP.md`,
  `ROADMAP.md`, etc). The actual code (`go-api/`, `python-cv/`, `sdk/`,
  `docs/`) had been uploaded as a zip (`fashion-tryon-mvp.zip`) which
  was then deleted without ever being committed as tracked files.
- Extracted the zip's contents from git history and re-committed them
  as normal tracked files. Codebase now actually matches what
  `CLAUDE.md`/`PROJECT_MAP.md` describe.

## 2026-09-09 — Clothing try-on pipeline: scaffolding + async job contract

What's real (runs today, CPU-only, no GPU needed):
- `python-cv/body_pose.py` — MediaPipe Pose + SelfieSegmentation. Given
  a customer's photo, extracts shoulder/hip/limb landmarks and a person
  segmentation mask. Mirrors `face_analysis.py`'s existing pattern.
- `python-cv/jobs.py` — in-memory async job store (create/poll/mark
  done-or-failed). Stand-in for Redis until this needs to run on more
  than one process.
- `POST /body/analyze`, `POST /tryon/clothing`, `GET
  /tryon/clothing/{job_id}` in `python-cv/main.py` — full async
  create-job → poll-job contract, testable end to end today.
- `go-api`: `TryOn` handler now routes `category: "clothing_*"` to an
  async job (reuses the customer's already-stored S3 photo via a
  presigned URL — never re-uploads), new `GET /api/v1/tryon/{jobId}`
  polls it.
- `sdk/tryon-widget.js` — polls the job endpoint instead of expecting a
  synchronous response when the API returns a `job_id`.

What's NOT real yet (the one deliberate gap):
- `python-cv/clothing_generation.py` — the actual diffusion model call
  (CatVTON or IDM-VTON). No GPU is available in this dev sandbox, so
  this function has a fully-specified interface but `raise
  NotImplementedError` instead of a real model call. Every
  `/tryon/clothing` job today ends with `status: "failed"` and that
  message — this is expected, not a bug, until a GPU worker exists.
- Result storage: once generation works, the generated image needs to
  land in S3 and the job needs a real `result_url`. Not wired — noted
  as a TODO in `main.py`.

Commits: `81430dc` (restore), `fde5920` (clothing pipeline). PR #1
(same as above) has both.

**Not yet verified**: `go build ./...` on `go-api` has not been run in
this session to confirm the Go changes actually compile — do that
before treating this as done.

## Next up

See `ROADMAP.md` Phase 2 remaining items: GCP GPU instance, benchmark
CatVTON vs IDM-VTON, wire the real model call into
`clothing_generation.py`, S3 result storage, ethnic-wear testing.
