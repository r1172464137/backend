#!/usr/bin/env python3
"""Static contract checks for stable-release test gates."""

from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parents[1]
WORKFLOW = ROOT / ".github" / "workflows" / "release.yml"
TEXT = WORKFLOW.read_text(encoding="utf-8")


def fail(message: str) -> None:
    print(f"FAIL: {message}", file=sys.stderr)
    raise SystemExit(1)


# Preserve the pre-existing REALITY compatibility gate.
if "TestRealityXver|TestNormalizeProtocol|TestSanitizeProtocols|TestMergeMissingProtocol|TestCompatible" not in TEXT:
    fail("stable release workflow lost the existing REALITY/protocol compatibility gate")

match = re.search(
    r"- name: Structured route-rule release gates\n\s+run: \|\n(?P<body>(?:\s{10}.*\n)+)",
    TEXT,
)
if not match:
    fail("stable release workflow has no structured route-rule gate")
body = match.group("body")
if "go test -race" not in body or "-count=1" not in body:
    fail("structured route-rule gate must run uncached race tests")

required_packages = {
    "./internal/config",
    "./internal/app/bootstrap",
    "./internal/app/migration/schema",
    "./internal/module/network/internal/adminserver",
    "./internal/module/network/internal/nodeconfig",
    "./internal/module/network/internal/repo",
    "./internal/module/network/internal/serverapi",
    "./internal/module/network/transport/http/admin/server",
    "./internal/module/network/transport/http/server",
    "./internal/module/platform/internal/repo",
    "./internal/module/platform/internal/systemsetting",
    "./internal/module/platform/transport/http/admin/system",
    "./internal/transport/http/routes",
}
missing = sorted(package for package in required_packages if package not in body)
if missing:
    fail(f"structured route-rule release gate is missing packages: {missing}")
if "python3 script/check-release-workflow-contract.py" not in body:
    fail("release workflow must execute its static contract checker")

print("Release workflow structured route-rule contract OK")
