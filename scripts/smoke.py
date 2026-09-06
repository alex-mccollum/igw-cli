"""Verify the Linux v1 executable in isolated configuration directories."""

import argparse
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import threading


TOKEN = "smoke:private-fixture-token"
ARTIFACT = b"igw smoke artifact\n" * 128


class Fixture(BaseHTTPRequestHandler):
    writes = 0

    def log_message(self, *_):
        pass

    def do_PUT(self):
        type(self).writes += 1
        self.send_response(500)
        self.end_headers()

    def do_GET(self):
        code, body = 200, b'{"ok":true,"number":9007199254740993}'
        if self.headers.get("X-Ignition-API-Token") != TOKEN:
            code, body = 401, b'{}'
        elif self.path == "/denied":
            code, body = 403, b'{}'
        elif self.path == "/failed":
            code, body = 500, b'{}'
        elif self.path == "/artifact":
            body = ARTIFACT
        self.send_response(code)
        self.send_header("Content-Type", "application/octet-stream" if self.path == "/artifact" else "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True)
    parser.add_argument("--live", action="store_true", help="also run explicit read-only Gateway checks")
    parser.add_argument("--profile")
    args = parser.parse_args()
    if not sys.platform.startswith("linux"):
        parser.error("isolated executable smoke currently requires Linux XDG configuration paths")
    binary = str(Path(args.binary).resolve())
    checks = []
    original_env = os.environ.copy()

    def run(name, command, env, token="", expected=0):
        process = subprocess.run([binary, *command, "--json"], input=token.encode(),
                                 capture_output=True, env=env, timeout=35)
        if process.returncode != expected or process.stderr or TOKEN.encode() in process.stdout:
            raise RuntimeError(f"{name}: unexpected exit/output (exit {process.returncode})")
        result = json.loads(process.stdout)
        if result.get("version") != "igw/v1" or result.get("ok") != (expected == 0):
            raise RuntimeError(f"{name}: invalid result contract")
        checks.append({"name": name, "exitCode": process.returncode})
        return result

    with tempfile.TemporaryDirectory(prefix="igw-smoke-") as root:
        env = original_env.copy()
        env.pop("IGNITION_GATEWAY_URL", None)
        env.pop("IGNITION_API_TOKEN", None)
        env["XDG_CONFIG_HOME"] = str(Path(root) / "config")
        env["XDG_CACHE_HOME"] = str(Path(root) / "cache")
        base = Path(env["XDG_CONFIG_HOME"]) / "igw"
        version = run("version", ["version"], env)
        if not version["data"].get("version"):
            raise RuntimeError("missing application version")
        for alias in ("version", "--version", "-v"):
            process = subprocess.run([binary, alias], capture_output=True, env=env, timeout=5)
            if process.returncode or not process.stdout.startswith(b"igw version "):
                raise RuntimeError("human version contract changed")
            checks.append({"name": "version alias " + alias, "exitCode": 0})
        if run("schema", ["schema"], env)["data"]["name"] != "igw":
            raise RuntimeError("wrong command tree")
        run("command schema", ["schema", "resource", "update"], env)
        run("exit codes", ["exit-codes"], env)
        run("group help", ["api"], env)
        if run("selected help", ["help", "api"], env)["data"]["name"] != "api":
            raise RuntimeError("help selected the wrong command")
        run("unknown help topic", ["help", "unknown"], env, expected=2)
        run("unknown nested help topic", ["help", "api", "unknown"], env, expected=2)
        run("unknown nested command", ["api", "unknown"], env, expected=2)
        run("removed RPC", ["rpc"], env, expected=2)
        run("empty profiles", ["profile", "list"], env)
        run("offline references", ["spec", "references", "list"], env)
        if base.exists():
            raise RuntimeError("offline discovery created configuration")
        server = ThreadingHTTPServer(("127.0.0.1", 0), Fixture)
        worker = threading.Thread(target=server.serve_forever, daemon=True)
        worker.start()
        try:
            target = f"http://127.0.0.1:{server.server_port}"
            setup = ["profile", "set", "local", "--url", target, "--use", "--token-stdin"]
            run("profile preview", [*setup, "--dry-run"], env, TOKEN)
            if base.exists():
                raise RuntimeError("preview created configuration")
            run("profile setup", [*setup, "--yes"], env, TOKEN)
            raw = ["api", "raw", "--method", "GET", "--path"]
            got = run("configured request", [*raw, "/ok"], env)
            if got["data"]["number"] != 9007199254740993:
                raise RuntimeError("JSON number was rounded")
            run("auth exit", [*raw, "/denied"], env, expected=6)
            run("HTTP exit", [*raw, "/failed"], env, expected=7)
            run("mutation confirmation", ["api", "raw", "--method", "PUT", "--path", "/mutation"], env, expected=2)
            run("mutation preview", ["api", "raw", "--method", "PUT", "--path", "/mutation", "--dry-run"], env)
            if Fixture.writes:
                raise RuntimeError("smoke preview sent a mutation")
            output = Path(root) / "artifact.bin"
            saved = run("artifact download", [*raw, "/artifact", "--out", str(output)], env)
            if output.read_bytes() != ARTIFACT or saved["artifact"]["sha256"] != hashlib.sha256(ARTIFACT).hexdigest():
                raise RuntimeError("artifact bytes/hash differ")
            run("artifact overwrite refusal", [*raw, "/artifact", "--out", str(output)], env, expected=2)
            if output.read_bytes() != ARTIFACT:
                raise RuntimeError("refused overwrite changed destination")
            revision = run("profile revision", ["profile", "list"], env)["data"]["revision"]
            run("clear token", ["profile", "set", "local", "--clear-token", "--yes"], env)
            run("stale profile revision", ["profile", "use", "--default", "--if-revision", revision, "--yes"], env, expected=2)
        finally:
            server.shutdown()
            server.server_close()
            worker.join(timeout=5)
        env["XDG_CONFIG_HOME"] = str(Path(root) / "migration")
        base = Path(env["XDG_CONFIG_HOME"]) / "igw"
        base.mkdir(parents=True, mode=0o700)
        legacy = json.dumps({"gatewayURL": "https://legacy.invalid", "token": TOKEN}).encode()
        legacy_path = base / "config.json"
        legacy_path.write_bytes(legacy)
        legacy_path.chmod(0o600)
        run("migration preview", ["profile", "migrate", "--dry-run"], env)
        run("migration apply", ["profile", "migrate", "--yes"], env)
        current = (base / "config.v1.json").read_bytes()
        revision = run("migration revision", ["profile", "list"], env)["data"]["revision"]
        run("rollback preview", ["profile", "rollback", "--if-revision", revision, "--dry-run"], env)
        back = run("rollback apply", ["profile", "rollback", "--if-revision", revision, "--yes"], env)
        if legacy_path.read_bytes() != legacy or (base / back["data"]["archive"]).read_bytes() != current:
            raise RuntimeError("rollback changed original bytes")
        if (base / "config.v1.json").exists():
            raise RuntimeError("rollback did not deactivate v1")
        process = subprocess.Popen([binary, "profile", "set", "waiting", "--token-stdin", "--yes", "--timeout", "20ms", "--json"],
                                   stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env)
        try:
            if process.wait(timeout=5) != 7 or json.loads(process.stdout.read())["error"]["kind"] != "timeout":
                raise RuntimeError("blocked stdin ignored deadline")
            checks.append({"name": "blocked stdin deadline", "exitCode": 7})
        finally:
            if process.poll() is None:
                process.kill()
                process.wait()
            process.stdin.close()
            process.stdout.close()
            process.stderr.close()
    if args.live:
        profile = ["--profile", args.profile] if args.profile else []
        for name, command in [("live doctor", ["gateway", "doctor"]), ("live catalog", ["spec", "sync"]),
                              ("live request", ["api", "request", "GET /data/api/v1/gateway-info"])]:
            run(name, [*command, *profile, "--timeout", "30s"], original_env)
    print(json.dumps({"version": "igw-smoke/1", "passed": True, "liveReads": args.live, "checks": checks}))


if __name__ == "__main__":
    try:
        main()
    except (OSError, subprocess.TimeoutExpired, ValueError, KeyError, RuntimeError) as error:
        print("smoke failed: " + str(error), file=sys.stderr)
        raise SystemExit(1)
