#!/usr/bin/env python3
"""Build one reviewable module-profile reference; never publish or recover hosts."""

import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import sys


ROOT = Path(__file__).resolve().parent.parent
DEFAULT_BASELINES = {
    "image-defaults": ROOT / "internal/reference/bundles/ignition-8.3.9-defaults/openapi.json.gz",
    "core-opcua": ROOT / "internal/reference/bundles/ignition-8.3.9-core/openapi.json.gz",
}
MAX_METADATA = 4 << 20
ENGINE_FORMAT = ('{"OSType":{{json .OSType}},"CgroupVersion":{{json .CgroupVersion}},'
                 '"MemoryLimit":{{json .MemoryLimit}},"SwapLimit":{{json .SwapLimit}},'
                 '"CPUCfsQuota":{{json .CPUCfsQuota}},"PidsLimit":{{json .PidsLimit}}}')


def now():
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def read_json(path):
    with path.open("rb") as source:
        raw = source.read(MAX_METADATA + 1)
    if len(raw) > MAX_METADATA:
        raise ValueError("metadata exceeds size limit")
    return json.loads(raw)


def publish_json(path, value):
    # Only the coordinator's new private directory is writable here.
    temporary = path.with_suffix(".pending")
    with temporary.open("w", encoding="utf-8") as output:
        json.dump(value, output, indent=2)
        output.write("\n")
        output.flush()
        os.fsync(output.fileno())
    temporary.replace(path)


def public_summary(receipt):
    """Publish control outcomes only; never copy diagnostics or arbitrary text."""
    def choice(value, allowed):
        if value not in allowed:
            raise ValueError("invalid public summary field")
        return value

    def matched(value, pattern):
        if not isinstance(value, str) or not re.fullmatch(pattern, value):
            raise ValueError("invalid public summary field")
        return value

    summary = {
        "version": "igw/reference-update-summary/v1",
        "status": choice(receipt["status"], ("qualified", "failed")),
        "tag": matched(receipt["tag"], r"8\.3(?:\.(?:0|[1-9][0-9]{0,4}))?"),
        "moduleProfile": choice(receipt["moduleProfile"], tuple(DEFAULT_BASELINES)),
        "steps": [],
    }
    if "source" in receipt:
        summary["sourceCommit"] = matched(receipt["source"]["commit"], r"[a-f0-9]{40,64}")
    if "image" in receipt:
        summary["image"] = matched(receipt["image"], r"inductiveautomation/ignition@sha256:[a-f0-9]{64}")
    stages = ("admission", "engine", "exclusive-slot", "source-commit", "source-status",
              "toolchain", "build-capture", "build-tests", "resolve", "pull", "lifecycle",
              "capture", "resources", "transfers", "operations", "verify-source-commit",
              "verify-source-status", "qualify")
    for step in receipt["steps"]:
        item = {"name": choice(step["name"], stages),
                "status": choice(step["status"], ("running", "passed", "failed", "interrupted"))}
        if "exitCode" in step:
            code = step["exitCode"]
            if type(code) is not int or not -255 <= code <= 255:
                raise ValueError("invalid public summary exit code")
            item["exitCode"] = code
        summary["steps"].append(item)
    return summary


class StageFailure(Exception):
    pass


class Update:
    def __init__(self, root, out, tag, docker, baseline, pull, require_clean=False, module_profile="image-defaults"):
        if module_profile not in ("image-defaults", "core-opcua"):
            raise ValueError("unsupported module profile")
        self.root, self.out = root, out
        self.tag, self.docker, self.baseline, self.pull = tag, docker, baseline, pull
        self.require_clean = require_clean
        self.module_profile = module_profile
        self.guard = root / "scripts/bounded-run.sh"
        self.receipt = {
            "version": "igw/reference-update/v1", "startedAt": now(),
            "status": "running", "tag": tag, "moduleProfile": module_profile,
            "pull": pull, "requireClean": require_clean, "steps": [],
        }

    def step(self, name, command, env=None, check=False):
        record = {"name": name, "startedAt": now(), "status": "running"}
        self.receipt["steps"].append(record)
        self.save()
        print("reference-update: " + name, flush=True)
        # Each stage receives its own verified resource scope. Building and
        # running containers never overlap, and a failed guard has no fallback.
        invocation = ["bash", str(self.guard)]
        invocation += ["--check"] if check else ["--", *map(str, command)]
        stage_env = os.environ.copy()
        # Ambient opt-in tests/artifact destinations must not change the plan.
        for key in list(stage_env):
            if key.startswith("IGW_"):
                del stage_env[key]
        stage_env.update(env or {})
        stage_env["TMPDIR"] = str(self.out / "temporary")
        stage_env["TMP"] = stage_env["TEMP"] = stage_env["TMPDIR"]
        stdout = self.out / "logs" / (name + ".stdout")
        stderr = self.out / "logs" / (name + ".stderr")
        with stdout.open("xb") as output, stderr.open("xb") as errors:
            process = subprocess.Popen(invocation, cwd=self.root, env=stage_env,
                                       stdout=output, stderr=errors, start_new_session=True)
            try:
                code = process.wait(timeout=660)
            except subprocess.TimeoutExpired:
                self.stop(process)
                code = 124
            except (KeyboardInterrupt, SystemExit):
                # Signal only this coordinator-owned process group. The guard
                # owns scope cleanup; containers also have independent limits.
                self.stop(process)
                record.update(finishedAt=now(), status="interrupted")
                self.save()
                raise
        record.update(finishedAt=now(), exitCode=code,
                      status="passed" if code == 0 else "failed")
        self.save()
        if code != 0:
            raise StageFailure("stage failed: " + name + "; inspect retained logs")
        return stdout

    @staticmethod
    def stop(process):
        if process.poll() is None:
            os.killpg(process.pid, signal.SIGTERM)
            try:
                process.wait(timeout=30)
            except subprocess.TimeoutExpired:
                # Only this private process group, never the engine or host.
                os.killpg(process.pid, signal.SIGKILL)
                process.wait(timeout=10)

    def save(self):
        publish_json(self.out / "run.json", self.receipt)

    def run(self):
        self.out.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        self.out.mkdir(mode=0o700)  # Existing candidates are never replaced.
        for name in ("logs", "tools", "temporary"):
            (self.out / name).mkdir(mode=0o700)
        try:
            self.step("admission", [], check=True)
            info_path = self.step("engine", [self.docker, "info", "--format", ENGINE_FORMAT])
            info = read_json(info_path)
            required = ("MemoryLimit", "SwapLimit", "CPUCfsQuota", "PidsLimit")
            if info.get("OSType") != "linux" or info.get("CgroupVersion") != "2" or not all(info.get(k) is True for k in required):
                raise StageFailure("Docker engine lacks required Linux cgroup v2 controls")
            existing = self.step("exclusive-slot", [self.docker, "ps", "--all", "--filter", "label=io.igw-cli.qualification", "--format", "{{.ID}}"])
            if existing.stat().st_size:
                raise StageFailure("a qualification container remains; inspect owned cleanup before continuing")

            source = self.step("source-commit", ["git", "rev-parse", "HEAD"])
            commit = source.read_text().strip()
            if not re.fullmatch(r"[a-f0-9]{40,64}", commit):
                raise StageFailure("cannot identify source commit")
            dirty = self.step("source-status", ["git", "status", "--porcelain", "--untracked-files=normal"])
            self.receipt["source"] = {"commit": commit, "dirty": bool(dirty.stat().st_size)}
            self.receipt["coordinatorSha256"] = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
            if self.require_clean and self.receipt["source"]["dirty"]:
                raise StageFailure("qualification requires a clean source checkout")

            toolchain = read_json(self.step("toolchain", ["go", "env", "-json", "GOVERSION", "GOOS", "GOARCH", "CGO_ENABLED"]))
            if toolchain.get("GOOS") != "linux" or toolchain.get("GOARCH") != "amd64" or not toolchain.get("GOVERSION"):
                raise StageFailure("qualification requires a Linux amd64 Go target")
            self.receipt["toolchain"] = toolchain

            capture = self.out / "tools/igw-capture"
            tests = self.out / "tools/testgateway.test"
            self.step("build-capture", ["go", "build", "-trimpath", "-o", capture, "./cmd/igw-capture"])
            self.step("build-tests", ["go", "test", "-c", "-trimpath", "-o", tests, "./internal/testgateway"])
            self.step("resolve", [capture, "resolve", "--tag", self.tag, "--out", self.out / "resolution"])
            resolution = read_json(self.out / "resolution/resolution.json")
            image = resolution.get("image", "")
            if not re.fullmatch(r"inductiveautomation/ignition@sha256:[a-f0-9]{64}", image):
                raise StageFailure("resolver did not return an immutable official image")
            self.receipt["image"] = image
            self.save()
            if self.pull:
                self.step("pull", [self.docker, "pull", "--platform", "linux/amd64", image])

            common = {"IGW_CAPTURE_TEST_DOCKER": self.docker,
                      "IGW_TEST_MODULE_PROFILE": self.module_profile}
            self.step("lifecycle", [tests, "-test.run", "^TestLiveCaptureLifetime$", "-test.v"], {
                **common, "IGW_CAPTURE_TEST_IMAGE": image,
                "IGW_LIFECYCLE_EVIDENCE": str(self.out / "lifecycle.json"),
            })
            self.step("capture", [capture, "--image", image, "--docker", self.docker,
                                  "--module-profile", self.module_profile, "--out", self.out / "capture"])
            for name, test, variable, receipt in (
                ("resources", "TestLiveAPIResourceContract", "IGW_ACCEPTANCE_EVIDENCE", "resource-workflows.json"),
                ("transfers", "TestLiveProjectTagWorkflows", "IGW_TRANSFER_EVIDENCE", "project-tag-workflows.json"),
                ("operations", "TestLiveOperationalWorkflows", "IGW_OPERATIONS_EVIDENCE", "operational-workflows.json"),
            ):
                self.step(name, [tests, "-test.run", "^" + test + "$", "-test.v"], {
                    **common, "IGW_ACCEPTANCE_TEST_IMAGE": image,
                    variable: str(self.out / receipt),
                })
            if self.require_clean:
                final_commit = self.step("verify-source-commit", ["git", "rev-parse", "HEAD"])
                final_status = self.step("verify-source-status", ["git", "status", "--porcelain", "--untracked-files=normal"])
                if final_commit.read_text().strip() != commit or final_status.stat().st_size:
                    raise StageFailure("source checkout changed during qualification")
            self.step("qualify", [capture, "qualify", "--resolution", self.out / "resolution",
                                  "--capture", self.out / "capture", "--lifecycle", self.out / "lifecycle.json",
                                  "--resources", self.out / "resource-workflows.json",
                                  "--transfers", self.out / "project-tag-workflows.json",
                                  "--operations", self.out / "operational-workflows.json",
                                  "--test-binary", tests, "--baseline", self.baseline,
                                  "--out", self.out / "reference"])
            # The Go qualifier verifies all evidence and reads back its bundle.
            manifest = read_json(self.out / "reference/reference.json")
            if manifest.get("moduleProfile", {}).get("name") != self.module_profile:
                raise StageFailure("qualified reference differs from the requested module profile")
            self.receipt.update(status="qualified", reference=manifest["name"],
                                catalog=manifest["catalog"], comparison=read_json(self.out / "reference/evidence/qualification.json")["comparison"],
                                moduleProfileEvidence=manifest["moduleProfile"],
                                testBinarySha256=manifest["qualification"]["testBinarySha256"])
        except BaseException as error:
            self.receipt["status"] = "failed"
            self.receipt["failure"] = {"kind": type(error).__name__, "message": str(error)}
            raise
        finally:
            self.receipt["finishedAt"] = now()
            self.save()
            publish_json(self.out / "public-run.json", public_summary(self.receipt))
        return self.receipt


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tag", default="8.3", help="Official 8.3 channel or explicit 8.3.patch tag")
    parser.add_argument("--out", required=True, type=Path, help="New private directory for run evidence and candidate")
    parser.add_argument("--docker", default="docker", help="Docker executable; the engine must already be running")
    parser.add_argument("--baseline", type=Path, help="Previous qualified JSON or JSON.gz; defaults to the matching 8.3.9 module profile")
    parser.add_argument("--skip-pull", action="store_true", help="Require the resolved image to be present locally")
    parser.add_argument("--require-clean", action="store_true", help="Require the source commit and clean checkout to stay unchanged")
    parser.add_argument("--module-profile", default="image-defaults", choices=("image-defaults", "core-opcua"), help="Reviewed module selection applied to every Gateway")
    args = parser.parse_args(argv)
    if args.baseline is None:
        args.baseline = DEFAULT_BASELINES[args.module_profile]
    if not re.fullmatch(r"8\.3(?:\.(?:0|[1-9][0-9]{0,4}))?", args.tag):
        parser.error("tag must be 8.3 or 8.3.patch")
    if sys.platform != "linux":
        parser.error("reference update requires a Linux runner with verified cgroup v2 limits")
    if not args.baseline.is_file():
        parser.error("baseline must be an existing qualified document")
    if args.out.exists() or args.out.is_symlink():
        parser.error("output directory already exists; preserve it and choose a new path")
    if not args.docker or args.docker.startswith("-"):
        parser.error("docker must name an executable")
    os.umask(0o077)
    signal.signal(signal.SIGTERM, lambda signum, frame: sys.exit(143))
    try:
        import fcntl
        runtime = Path("/run/user") / str(os.getuid())
        if runtime.is_symlink() or runtime.stat().st_uid != os.getuid():
            raise StageFailure("private user runtime directory unavailable")
        with (runtime / "igw-reference-update.lock").open("a") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            Update(ROOT, args.out.absolute(), args.tag, args.docker,
                   args.baseline.absolute(), not args.skip_pull, args.require_clean, args.module_profile).run()
    except (OSError, ValueError, KeyError, StageFailure, subprocess.TimeoutExpired) as error:
        print("reference-update: " + str(error), file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        return 130
    print("reference-update: qualified candidate retained; review is required before distribution")
    return 0


if __name__ == "__main__":
    sys.exit(main())
