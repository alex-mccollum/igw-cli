"""Status contracts with synthetic API responses; no GitHub or Gateway access."""

from contextlib import redirect_stdout
from copy import deepcopy
from datetime import datetime, timedelta, timezone
from http.client import IncompleteRead
import importlib.util
import io
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
from urllib.error import HTTPError


SPEC = importlib.util.spec_from_file_location("reference_status", Path(__file__).with_name("reference-status.py"))
STATUS = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(STATUS)
NOW = datetime(2026, 9, 6, 20, tzinfo=timezone.utc)
REPO = "owner/repo"


def run(number, conclusion="success", attempt=1):
    return {"id": number, "status": "completed", "conclusion": conclusion,
            "created_at": NOW.isoformat(), "run_attempt": attempt,
            "head_branch": "main", "event": "schedule", "path": ".github/workflows/reference-update.yml"}


def jobs(finished=NOW):
    return [{"name": "Qualify (" + profile + ")", "conclusion": "success",
             "completed_at": finished.isoformat(), "steps": [
                 {"name": name, "conclusion": "success"}
                 for name in (STATUS.QUALIFY_STEP, STATUS.RETAIN_STEP)]}
            for profile in STATUS.PROFILES]


class FakeAPI:
    def __init__(self, runs, jobsets=None, state="active"):
        self.runs, self.jobsets, self.state = runs, jobsets or {}, state
        self.paths = []

    def get(self, path, missing_ok=False):
        self.paths.append(path)
        if path == "":
            return {"default_branch": "main"}
        if path == "/actions/workflows/reference-update.yml":
            return None if self.state is None else {"state": self.state}
        if path.startswith("/actions/workflows/reference-update.yml/runs?"):
            return {"workflow_runs": deepcopy(self.runs)}
        if "/attempts/" in path:
            parts = path.split("/")
            selected = self.jobsets[(int(parts[3]), int(parts[5]))]
            return {"total_count": len(selected), "jobs": deepcopy(selected)}
        if path.startswith("/actions/runs/"):
            return deepcopy(next(r for r in self.runs if r["id"] == int(path.split("/")[-1])))
        raise AssertionError("unexpected API request: " + path)


class StatusTests(unittest.TestCase):
    def collect(self, api, **kwargs):
        return STATUS.collect(api, REPO, now=NOW, **kwargs)

    def test_complete_qualification_and_summary(self):
        report = self.collect(FakeAPI([run(2)], {(2, 1): jobs()}), enabled="true")
        self.assertEqual(report["status"], "qualified")
        self.assertEqual(report["lastQualification"]["id"], 2)
        self.assertIn("Last complete qualification", STATUS.markdown(report))

    def test_skipped_green_run_does_not_hide_last_success(self):
        skipped = jobs()
        skipped[0]["conclusion"] = "skipped"
        report = self.collect(FakeAPI([run(2), run(1)], {(2, 1): skipped, (1, 1): jobs()}))
        self.assertEqual(report["status"], "unverified")
        self.assertEqual(report["lastQualification"]["id"], 1)

    def test_failure_preserves_last_qualification(self):
        report = self.collect(FakeAPI([run(2, "failure"), run(1)], {(2, 1): [], (1, 1): jobs()}))
        self.assertEqual(report["status"], "failed")
        self.assertEqual(report["lastQualification"]["id"], 1)
        self.assertIn("preserve the last good", " ".join(report["warnings"]))

    def test_failed_status_job_does_not_erase_qualification(self):
        report = self.collect(FakeAPI([run(2, "failure")], {(2, 1): jobs()}))
        self.assertEqual(report["status"], "failed")
        self.assertEqual(report["lastQualification"]["id"], 2)

    def test_missing_disabled_and_unknown_activation(self):
        for state, enabled, expected in [(None, "true", "not-installed"), ("disabled_manually", "true", "inactive"), ("active", "false", "inactive"), ("active", "unknown", "unverified")]:
            with self.subTest(state=state, enabled=enabled):
                report = self.collect(FakeAPI([], state=state), enabled=enabled)
                self.assertEqual(report["status"], expected)
                self.assertIsNone(report["lastQualification"])

    def test_skipped_or_missing_step_and_failed_upload_are_not_evidence(self):
        for mutation in ("skip", "missing", "upload", "profile"):
            selected = jobs()
            if mutation == "skip":
                selected[0]["steps"][0]["conclusion"] = "skipped"
            elif mutation == "missing":
                selected[0]["steps"] = []
            elif mutation == "upload":
                selected[1]["steps"][1]["conclusion"] = "failure"
            else:
                selected.pop()
            report = self.collect(FakeAPI([run(1)], {(1, 1): selected}))
            self.assertIsNone(report["lastQualification"], mutation)

    def test_overdue_and_future_completion(self):
        report = self.collect(FakeAPI([run(1)], {(1, 1): jobs(NOW - timedelta(days=15))}))
        self.assertEqual(report["status"], "overdue")
        with self.assertRaises(STATUS.StatusError):
            self.collect(FakeAPI([run(1)], {(1, 1): jobs(NOW + timedelta(seconds=1))}))

    def test_current_run_finishes_qualification_before_status_job(self):
        current = run(2, None)
        current["status"] = "in_progress"
        api = FakeAPI([current], {(2, 1): jobs()})
        report = self.collect(api, enabled="true", current_run=2, current_result="success")
        self.assertEqual(report["status"], "qualified")
        self.assertEqual(report["lastQualification"]["id"], 2)
        report = self.collect(api, enabled="false", current_run=2, current_result="skipped")
        self.assertEqual(report["status"], "inactive")

    def test_attempt_isolation_and_branch_filter(self):
        api = FakeAPI([run(2, attempt=2)], {(2, 1): jobs(), (2, 2): jobs()[:1]})
        report = self.collect(api)
        self.assertIsNone(report["lastQualification"])
        self.assertTrue(any("/attempts/2/" in path for path in api.paths))
        other = run(1)
        other["head_branch"] = "untrusted"
        self.assertIsNone(self.collect(FakeAPI([other]))["latestRun"])
        with self.assertRaises(STATUS.StatusError):
            self.collect(FakeAPI([other]), current_run=1, current_result="success")

    def test_pending_run_does_not_claim_fresh_completion(self):
        pending = run(2, None)
        pending["status"] = "queued"
        report = self.collect(FakeAPI([pending, run(1)], {(1, 1): jobs()}))
        self.assertEqual(report["status"], "pending")
        self.assertEqual(report["lastQualification"]["id"], 1)

    def test_newest_completion_can_be_an_older_run_rerun(self):
        api = FakeAPI([run(2), run(1, attempt=2)], {
            (2, 1): jobs(NOW - timedelta(days=1)), (1, 2): jobs()})
        report = self.collect(api)
        self.assertEqual(report["status"], "qualified")
        self.assertEqual(report["lastQualification"]["id"], 1)

    def test_incomplete_job_response_fails_closed(self):
        with patch.object(FakeAPI, "get", return_value={"total_count": 3, "jobs": jobs()}):
            with self.assertRaises(STATUS.StatusError):
                STATUS.qualified_at(FakeAPI([]), run(1))

    def test_main_reports_unavailable_without_leaking_errors(self):
        with tempfile.TemporaryDirectory() as directory:
            summary = Path(directory) / "summary.md"
            output = io.StringIO()
            with patch.object(STATUS.GitHub, "get", side_effect=ValueError("private-token")), patch.dict(os.environ, {"GITHUB_STEP_SUMMARY": str(summary)}), redirect_stdout(output):
                code = STATUS.main(["--repo", REPO, "--json"])
            self.assertEqual(code, 1)
            self.assertEqual(json.loads(output.getvalue())["status"], "unavailable")
            self.assertNotIn("private-token", output.getvalue() + summary.read_text())

    def test_http_errors_are_sanitized_and_only_workflow_404_is_missing(self):
        api = STATUS.GitHub(REPO, "private-token")
        with patch.object(STATUS, "urlopen", side_effect=HTTPError("private-url", 401, "private-token", {}, None)):
            with self.assertRaisesRegex(STATUS.StatusError, "^GitHub API returned HTTP 401$"):
                api.get("")

        with patch.object(STATUS, "urlopen", side_effect=HTTPError("private-url", 404, "private-token", {}, None)):
            self.assertIsNone(api.get("/actions/workflows/reference-update.yml", missing_ok=True))
            with self.assertRaises(STATUS.StatusError):
                api.get("")

    def test_interrupted_http_response_is_unavailable(self):
        api = STATUS.GitHub(REPO, "private-token")
        with patch.object(STATUS, "urlopen", side_effect=IncompleteRead(b"private-response", 100)):
            with self.assertRaisesRegex(STATUS.StatusError, "^GitHub API unavailable; status could not be verified$"):
                api.get("")


if __name__ == "__main__":
    unittest.main()
