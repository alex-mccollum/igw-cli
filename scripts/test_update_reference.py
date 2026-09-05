"""Coordinator contracts only: no Go compilation, registry, or Docker access."""

import importlib.util
import json
import os
from pathlib import Path
import signal
import subprocess
import tempfile
import unittest
from unittest.mock import patch


SPEC = importlib.util.spec_from_file_location("update_reference", Path(__file__).with_name("update-reference.py"))
UPDATE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(UPDATE)
IMAGE = "inductiveautomation/ignition@sha256:" + "b" * 64
COMMIT = "a" * 40
STEPS = ["admission", "engine", "exclusive-slot", "source-commit", "source-status", "toolchain",
         "build-capture", "build-tests", "resolve", "pull", "lifecycle", "capture",
         "resources", "transfers", "operations", "verify-source-commit",
         "verify-source-status", "qualify"]


class FakeProcess:
    pid = 12345

    def __init__(self, code=0, timeout=False, interrupt=False):
        self.code, self.timeout, self.interrupt = code, timeout, interrupt
        self.finished = False

    def wait(self, timeout=None):
        if self.timeout:
            self.timeout = False
            raise subprocess.TimeoutExpired("fake", timeout)
        if self.interrupt:
            self.interrupt = False
            raise KeyboardInterrupt
        self.finished = True
        return self.code

    def poll(self):
        return self.code if self.finished else None


class CoordinatorTests(unittest.TestCase):
    def setUp(self):
        self.scratch = tempfile.TemporaryDirectory()
        self.addCleanup(self.scratch.cleanup)
        self.root = Path(self.scratch.name)
        self.out = self.root / "candidate"
        self.seen = []
        self.failure = None
        self.dirty = None
        self.engine = {"OSType": "linux", "CgroupVersion": "2", "MemoryLimit": True,
                       "SwapLimit": True, "CPUCfsQuota": True, "PidsLimit": True}
        self.existing = ""
        self.timed_out = None
        self.interrupted = None
        self.previous = None
        self.profile = "image-defaults"
        self.manifest_profile = None

    def fake_popen(self, args, **kwargs):
        if self.previous:
            self.assertTrue(self.previous.finished, "overlapping stages")
        name = Path(kwargs["stdout"].name).stem
        self.assertEqual(args[:2], ["bash", str(self.root / "scripts/bounded-run.sh")])
        self.assertEqual(args[2], "--check" if name == "admission" else "--")
        self.assertTrue(kwargs["start_new_session"])
        self.assertEqual(kwargs["cwd"], self.root)
        self.assertNotIn("IGW_OPERATIONS_ARTIFACTS", kwargs["env"])
        if name not in ("lifecycle", "resources", "transfers", "operations"):
            self.assertNotIn("IGW_TEST_MODULE_PROFILE", kwargs["env"])
        self.assertEqual(kwargs["env"]["TMPDIR"], str(self.out / "temporary"))
        self.seen.append((name, args[3:], kwargs["env"]))
        output = ""
        if name == "engine":
            self.assertEqual(args[-1], UPDATE.ENGINE_FORMAT)
            self.assertNotIn("{{json .}}", args[-1])
            output = json.dumps(self.engine)
        if name == "exclusive-slot":
            output = self.existing
        if name == "toolchain":
            output = json.dumps({"GOVERSION": "go1.27.1", "GOOS": "linux", "GOARCH": "amd64", "CGO_ENABLED": "1"})
        if name in ("source-commit", "verify-source-commit"):
            output = COMMIT + "\n"
        if name == self.dirty:
            output = " M source.go\n"
        if name == "resolve":
            (self.out / "resolution").mkdir()
            (self.out / "resolution/resolution.json").write_text(json.dumps({"image": IMAGE}))
        if name == "qualify" and self.failure != name:
            (self.out / "reference").mkdir()
            (self.out / "reference/reference.json").write_text(json.dumps({
                "name": "fixture-reference", "catalog": {"contractSha256": "c" * 64},
                "comparison": {"contractEqual": True},
                "qualification": {"testBinarySha256": "d" * 64},
                "moduleProfile": {"policy": "igw-module-profile/1", "name": self.manifest_profile or self.profile},
            }))
        kwargs["stdout"].write(output.encode())
        self.previous = FakeProcess(7 if name == self.failure else 0,
                                    name == self.timed_out, name == self.interrupted)
        return self.previous

    def run_update(self, pull=True, clean=True):
        with patch.object(UPDATE.subprocess, "Popen", self.fake_popen), patch.dict(os.environ, {
            "IGW_OPERATIONS_ARTIFACTS": "must-not-inherit",
            "IGW_ACCEPTANCE_TEST_IMAGE": "must-not-inherit",
            "IGW_TEST_MODULE_PROFILE": "must-not-inherit",
        }):
            return UPDATE.Update(self.root, self.out, "8.3", "/docker path/docker.exe",
                                 self.root / "baseline.gz", pull, clean, self.profile).run()

    def receipt(self):
        return json.loads((self.out / "run.json").read_text())

    def test_serial_pinned_pipeline_and_receipt(self):
        receipt = self.run_update()
        self.assertEqual([item[0] for item in self.seen], STEPS)
        self.assertEqual(receipt["status"], "qualified")
        self.assertEqual(receipt["image"], IMAGE)
        self.assertEqual(receipt["source"], {"commit": COMMIT, "dirty": False})
        self.assertEqual(receipt["toolchain"]["GOVERSION"], "go1.27.1")
        self.assertTrue(all(s["status"] == "passed" for s in receipt["steps"]))
        test_paths = []
        for name, args, env in self.seen:
            if name in ("lifecycle", "resources", "transfers", "operations"):
                test_paths.append(args[0])
                self.assertEqual(env["IGW_CAPTURE_TEST_DOCKER"], "/docker path/docker.exe")
                self.assertEqual(env["IGW_TEST_MODULE_PROFILE"], "image-defaults")
                self.assertEqual(env.get("IGW_ACCEPTANCE_TEST_IMAGE", env.get("IGW_CAPTURE_TEST_IMAGE")), IMAGE)
            elif name != "admission":
                self.assertNotIn("IGW_ACCEPTANCE_TEST_IMAGE", env)
            if name in ("pull", "capture"):
                self.assertIn(IMAGE, args)
            if name == "qualify":
                self.assertEqual(args[args.index("--test-binary") + 1], str(self.out / "tools/testgateway.test"))
        self.assertEqual(len(set(test_paths)), 1)

    def test_core_selection_reaches_every_gateway_and_receipt(self):
        self.profile = "core-opcua"
        receipt = self.run_update()
        self.assertEqual(receipt["moduleProfile"], self.profile)
        self.assertEqual(receipt["moduleProfileEvidence"]["name"], self.profile)
        for name, args, env in self.seen:
            if name in ("lifecycle", "resources", "transfers", "operations"):
                self.assertEqual(env["IGW_TEST_MODULE_PROFILE"], self.profile)
            if name == "capture":
                self.assertEqual(args[args.index("--module-profile") + 1], self.profile)

    def test_qualified_profile_cannot_be_substituted(self):
        self.profile = "core-opcua"
        self.manifest_profile = "image-defaults"
        with self.assertRaises(UPDATE.StageFailure):
            self.run_update()
        self.assertEqual(self.receipt()["status"], "failed")
        self.assertNotIn("moduleProfileEvidence", self.receipt())

    def test_every_stage_failure_stops_and_preserves_evidence(self):
        for index, name in enumerate(STEPS):
            with self.subTest(stage=name):
                self.out = self.root / ("failed-" + name)
                self.seen, self.previous, self.failure = [], None, name
                with self.assertRaises(UPDATE.StageFailure):
                    self.run_update()
                self.assertEqual([item[0] for item in self.seen], STEPS[:index + 1])
                receipt = self.receipt()
                self.assertEqual(receipt["status"], "failed")
                self.assertEqual(receipt["steps"][-1]["exitCode"], 7)
                self.assertFalse((self.out / "reference/reference.json").exists())

    def test_preflight_refuses_unsupported_engine_and_existing_container(self):
        for case in ("controls", "existing"):
            with self.subTest(case=case):
                self.out = self.root / case
                self.seen, self.previous = [], None
                self.engine["SwapLimit"] = case != "controls"
                self.existing = "owned-container\n" if case == "existing" else ""
                with self.assertRaises(UPDATE.StageFailure):
                    self.run_update()
                self.assertNotIn("build-capture", [item[0] for item in self.seen])
                self.assertEqual(self.receipt()["status"], "failed")

    def test_dirty_or_changed_source_cannot_qualify(self):
        for stage in ("source-status", "verify-source-status"):
            self.out = self.root / stage
            self.seen, self.previous, self.dirty = [], None, stage
            with self.assertRaises(UPDATE.StageFailure):
                self.run_update()
            self.assertNotIn("qualify", [item[0] for item in self.seen])
            self.assertEqual(self.receipt()["status"], "failed")

    def test_optional_pull_and_no_clobber(self):
        self.run_update(pull=False)
        self.assertNotIn("pull", [item[0] for item in self.seen])
        original = (self.out / "run.json").read_bytes()
        with self.assertRaises(FileExistsError):
            self.run_update()
        self.assertEqual((self.out / "run.json").read_bytes(), original)

    def test_timeout_and_interrupt_stop_owned_group_without_retry(self):
        for case in ("timeout", "interrupt"):
            self.out = self.root / case
            self.seen, self.previous = [], None
            self.timed_out = "capture" if case == "timeout" else None
            self.interrupted = "capture" if case == "interrupt" else None
            with patch.object(UPDATE.os, "killpg") as stop:
                with self.assertRaises(UPDATE.StageFailure if case == "timeout" else KeyboardInterrupt):
                    self.run_update()
                stop.assert_called_once_with(FakeProcess.pid, signal.SIGTERM)
            self.assertEqual(self.seen[-1][0], "capture")
            self.assertEqual(self.receipt()["status"], "failed")


if __name__ == "__main__":
    unittest.main()
