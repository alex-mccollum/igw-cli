#!/usr/bin/env python3
"""Read hosted qualification status; never change references, runners, or settings."""

import argparse
from datetime import datetime, timedelta, timezone
from http.client import HTTPException
import json
import os
from pathlib import Path
import re
import sys
from urllib.error import HTTPError, URLError
from urllib.parse import urlencode
from urllib.request import Request, urlopen


WORKFLOW = "reference-update.yml"
PROFILES = ("image-defaults", "core-opcua")
QUALIFY_STEP = "Qualify resolved upstream image"
RETAIN_STEP = "Retain qualified API reference"
HISTORY_LIMIT = 20
MAX_RESPONSE = 4 << 20
OVERDUE_DAYS = 14


class StatusError(Exception):
    pass


class GitHub:
    def __init__(self, repo, token=""):
        self.root = "https://api.github.com/repos/" + repo
        self.token = token

    def get(self, path, missing_ok=False):
        headers = {"Accept": "application/vnd.github+json", "User-Agent": "igw-reference-status"}
        if self.token:
            headers["Authorization"] = "Bearer " + self.token
        try:
            with urlopen(Request(self.root + path, headers=headers), timeout=10) as response:
                raw = response.read(MAX_RESPONSE + 1)
            if len(raw) > MAX_RESPONSE:
                raise StatusError("GitHub response exceeds the status size limit")
            return json.loads(raw)
        except HTTPError as error:
            error.close()
            if error.code == 404 and missing_ok:
                return None
            raise StatusError("GitHub API returned HTTP " + str(error.code)) from None
        except (URLError, OSError, HTTPException):
            raise StatusError("GitHub API unavailable; status could not be verified") from None


def timestamp(value):
    parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    if parsed.tzinfo is None:
        raise ValueError("timestamp requires a timezone")
    return parsed


def run_summary(repo, run):
    return {"id": int(run["id"]), "status": run["status"], "conclusion": run["conclusion"],
            "url": f"https://github.com/{repo}/actions/runs/{int(run['id'])}",
            "createdAt": run["created_at"], "attempt": int(run["run_attempt"])}


def qualified_at(api, run):
    # Query the exact attempt: a partial rerun cannot borrow another attempt's
    # successful profile. A green workflow with skipped jobs is not evidence.
    attempt = int(run["run_attempt"])
    data = api.get(f"/actions/runs/{int(run['id'])}/attempts/{attempt}/jobs?per_page=100")
    jobs = data["jobs"]
    if data["total_count"] != len(jobs):
        raise StatusError("incomplete job listing; qualification could not be verified")
    finished = []
    for profile in PROFILES:
        matches = [job for job in jobs if job["name"] == f"Qualify ({profile})"]
        if len(matches) != 1 or matches[0]["conclusion"] != "success":
            return None
        job = matches[0]
        for name in (QUALIFY_STEP, RETAIN_STEP):
            steps = [step for step in job["steps"] if step["name"] == name]
            if len(steps) != 1 or steps[0]["conclusion"] != "success":
                return None
        finished.append(timestamp(job["completed_at"]))
    return max(finished)


def collect(api, repo, enabled="unknown", current_run=None, current_result=None, now=None):
    now = now or datetime.now(timezone.utc)
    report = {"version": "igw/reference-status/v1", "repository": repo,
              "checkedAt": now.isoformat(), "status": "unverified",
              "runnerEnabled": enabled, "latestRun": None, "lastQualification": None,
              "historyLimit": HISTORY_LIMIT, "overdueDays": OVERDUE_DAYS, "warnings": []}
    metadata = api.get("")  # A missing repository is not a missing workflow.
    workflow = api.get("/actions/workflows/" + WORKFLOW, missing_ok=True)
    if workflow is None:
        report["status"] = "not-installed"
        report["warnings"].append("Publish the reference workflow on the default branch before activation.")
        return report
    report["workflowState"] = workflow["state"]
    branch = metadata["default_branch"]
    query = urlencode({"branch": branch, "per_page": HISTORY_LIMIT})
    runs = api.get("/actions/workflows/" + WORKFLOW + "/runs?" + query)["workflow_runs"]
    if current_run is not None:
        current = api.get(f"/actions/runs/{int(current_run)}")
        if current["head_branch"] != branch or current["path"] != ".github/workflows/" + WORKFLOW or current["event"] not in ("schedule", "workflow_dispatch"):
            raise StatusError("current run is not a default-branch reference qualification")
        runs = [current] + [run for run in runs if run["id"] != current["id"]]
    runs = [run for run in runs if run["head_branch"] == branch
            and run["path"] == ".github/workflows/" + WORKFLOW
            and run["event"] in ("schedule", "workflow_dispatch")]
    if runs:
        report["latestRun"] = run_summary(repo, runs[0])
    if current_run is not None:
        report["currentQualificationResult"] = current_result
    qualified_runs = set()
    for run in runs:
        # A status-reporting failure must not erase completed qualification.
        # Inspect the profile jobs even when the overall workflow failed.
        if run["status"] != "completed" and not (run["id"] == current_run and current_result == "success"):
            continue
        finished = qualified_at(api, run)
        if finished is not None:
            qualified_runs.add(run["id"])
            if finished > now:
                raise StatusError("qualification completion date is in the future")
            prior = report["lastQualification"]
            if prior is None or finished > timestamp(prior["finishedAt"]):
                report["lastQualification"] = {**run_summary(repo, run), "finishedAt": finished.isoformat()}
    last = report["lastQualification"]
    if last is None:
        report["warnings"].append(f"No complete two-profile qualification found in the last {HISTORY_LIMIT} runs.")
    elif now - timestamp(last["finishedAt"]) > timedelta(days=OVERDUE_DAYS):
        report["status"] = "overdue"
        report["warnings"].append("Last qualification is older than two weekly intervals; retained references remain available.")
    else:
        report["status"] = "qualified"
    latest = report["latestRun"]
    outcome = current_result if current_run is not None else latest and latest["conclusion"]
    if outcome in ("failure", "cancelled", "timed_out", "action_required", "startup_failure"):
        report["status"] = "failed"
        report["warnings"].append("Latest workflow failed or was interrupted; inspect its run and preserve the last good references.")
    elif outcome == "skipped" or latest and latest["conclusion"] == "success" and latest["id"] not in qualified_runs:
        report["status"] = "unverified"
        report["warnings"].append("Latest run does not prove complete qualification of both profiles.")
    elif latest and latest["status"] != "completed" and latest["id"] not in qualified_runs:
        report["status"] = "pending"
        report["warnings"].append("Latest run is queued or in progress; a recent past success does not prove this run completed.")
    if workflow["state"] != "active" or enabled == "false":
        report["status"] = "inactive"
        report["warnings"].append("Workflow or dedicated runner gate is disabled; scheduled refresh is inactive.")
    elif enabled == "unknown":
        report["warnings"].append("Runner activation is unknown; qualification history does not verify runner availability.")
    return report


def markdown(report):
    lines = ["## API reference updater", "", "Status: **" + report["status"] + "**", "",
             "Runner gate: " + report["runnerEnabled"] + ".", ""]
    if report.get("currentQualificationResult"):
        lines += ["This run's qualification: " + report["currentQualificationResult"] + ".", ""]
    if report.get("latestRun"):
        lines += ["[Latest run](" + report["latestRun"]["url"] + ").", ""]
    last = report.get("lastQualification")
    if last:
        lines += ["Last complete qualification: [" + last["finishedAt"] + "](" + last["url"] + ").", ""]
    lines += ["- " + warning for warning in report["warnings"]]
    lines += ["", "Qualification produces a reviewable candidate; bundled references change only after review.", ""]
    return "\n".join(lines)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", default=os.environ.get("GITHUB_REPOSITORY", "alex-mccollum/igw-cli"))
    parser.add_argument("--json", action="store_true")
    parser.add_argument("--runner-enabled", choices=("true", "false", "unknown"), default="unknown")
    parser.add_argument("--current-run", type=int)
    parser.add_argument("--qualification-result", choices=("success", "failure", "cancelled", "skipped"))
    args = parser.parse_args(argv)
    if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", args.repo):
        parser.error("repo must be owner/name")
    if (args.current_run is None) != (args.qualification_result is None) or args.current_run is not None and args.current_run <= 0:
        parser.error("a positive current-run and qualification-result must be supplied together")
    token = os.environ.get("GH_TOKEN") or os.environ.get("GITHUB_TOKEN", "")
    try:
        report = collect(GitHub(args.repo, token), args.repo, args.runner_enabled,
                         args.current_run, args.qualification_result)
    except (StatusError, ValueError, KeyError, TypeError) as error:
        # Never echo remote response bodies, paths, or credentials.
        report = {"version": "igw/reference-status/v1", "repository": args.repo,
                  "status": "unavailable", "runnerEnabled": args.runner_enabled,
                  "warnings": [str(error) if isinstance(error, StatusError) else "Invalid GitHub status response."]}
    rendered = markdown(report)
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        with Path(os.environ["GITHUB_STEP_SUMMARY"]).open("a", encoding="utf-8") as output:
            output.write(rendered)
    print(json.dumps(report, indent=2) if args.json else rendered)
    return 0 if report["status"] == "qualified" else 1


if __name__ == "__main__":
    sys.exit(main())
