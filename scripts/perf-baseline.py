"""Measure actual v1 process latency without recording Gateway data or tokens."""

import argparse
import json
import os
from pathlib import Path
import subprocess
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True)
    parser.add_argument("--iterations", type=int, default=5)
    parser.add_argument("--live", action="store_true")
    parser.add_argument("--profile")
    args = parser.parse_args()
    if not 1 <= args.iterations <= 100:
        parser.error("iterations must be within 1..100")
    binary = str(Path(args.binary).resolve())
    commands = [
        ("command.schema", ["schema"]),
        ("reference.describe", ["api", "describe", "GET /data/api/v1/gateway-info", "--reference", "ignition-8.3.9-defaults"]),
    ]
    profile = ["--profile", args.profile] if args.profile else []
    if args.live:
        commands.extend([
            ("gateway.doctor", ["gateway", "doctor", *profile]),
            ("api.request", ["api", "request", "GET /data/api/v1/gateway-info", *profile]),
            ("api.batch.read", ["api", "batch", "--input", '[{"id":"status","operation":"GET /data/api/v1/gateway-info"}]', "--yes", *profile]),
        ])
    results = []
    failed = False
    for name, command in commands:
        samples, failures = [], []
        for _ in range(args.iterations):
            started = time.monotonic_ns()
            try:
                process = subprocess.run([binary, *command, "--json", "--timeout", "30s"],
                                         stdin=subprocess.DEVNULL, capture_output=True,
                                         env=os.environ.copy(), timeout=35)
                elapsed = (time.monotonic_ns() - started) / 1_000_000
                result = json.loads(process.stdout)
                if process.returncode or result.get("version") != "igw/v1" or not result.get("ok"):
                    failures.append({"exitCode": process.returncode})
                else:
                    samples.append(elapsed)
            except (OSError, subprocess.TimeoutExpired, ValueError):
                failures.append({"observationFailed": True})
        item = {"name": name, "iterations": args.iterations, "success": len(samples), "failures": failures}
        if samples:
            samples.sort()
            item.update(minMs=samples[0], p50Ms=samples[(len(samples)-1)//2],
                        p95Ms=samples[(len(samples)*95+99)//100-1], maxMs=samples[-1])
        failed |= bool(failures)
        results.append(item)
    print(json.dumps({"version": "igw-perf/1", "liveReads": args.live, "results": results}))
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
