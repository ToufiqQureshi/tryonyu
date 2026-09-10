"""
In-memory async job store for clothing try-on generation.

Clothing generation is slow (15-60s per ROADMAP.md's research), so the
`/tryon/clothing` contract is async: create a job, return its id
immediately, let the caller poll. This in-memory dict is a stand-in for
the real queue — it works for a single-process dev/demo deployment but
does NOT survive a restart and does NOT work across multiple go-api/
python-cv replicas. Swap for Redis (Streams, or just SET/GET job state)
once this needs to run on more than one process — see ROADMAP.md's
"Add a GPU worker queue" item.
"""

import threading
import uuid
from dataclasses import dataclass, field
from typing import Optional


@dataclass
class Job:
    id: str
    status: str = "processing"  # "processing" | "done" | "failed"
    result_url: Optional[str] = None
    error: Optional[str] = None


class JobStore:
    def __init__(self):
        self._jobs: dict[str, Job] = {}
        self._lock = threading.Lock()

    def create(self) -> Job:
        job = Job(id=str(uuid.uuid4()))
        with self._lock:
            self._jobs[job.id] = job
        return job

    def get(self, job_id: str) -> Optional[Job]:
        with self._lock:
            return self._jobs.get(job_id)

    def mark_done(self, job_id: str, result_url: str) -> None:
        with self._lock:
            job = self._jobs.get(job_id)
            if job:
                job.status = "done"
                job.result_url = result_url

    def mark_failed(self, job_id: str, error: str) -> None:
        with self._lock:
            job = self._jobs.get(job_id)
            if job:
                job.status = "failed"
                job.error = error


# Single process-wide store — fine for the current single-instance dev
# setup, see the module docstring for what changes once that's no longer
# true.
store = JobStore()
