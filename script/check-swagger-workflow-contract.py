#!/usr/bin/env python3
"""Static contract checks for the Swagger sync GitHub Actions workflow."""

from __future__ import annotations

import re
import sys
from pathlib import Path
from typing import cast

ROOT = Path(__file__).resolve().parents[1]
WORKFLOW = ROOT / ".github" / "workflows" / "swagger.yaml"
TEXT = WORKFLOW.read_text(encoding="utf-8")


def fail(message: str) -> None:
    print(f"FAIL: {message}", file=sys.stderr)
    sys.exit(1)


def require(pattern: str, message: str, *, flags: int = 0) -> re.Match[str]:
    match = re.search(pattern, TEXT, flags)
    if not match:
        fail(message)
    return cast(re.Match[str], match)


# PRs and all long-lived branches generate/verify, including forks.
require(r"pull_request:[\s\S]*branches:\s*\[master, dev, main\]", "pull_request must run verification for master, dev, and main")
require(r"push:[\s\S]*branches:\s*\[master, dev, main\]", "push must run verification for master, dev, and main")

# Check every relevant step, not just the first matching guard anywhere in YAML.
# Keep this checker dependency-free, as it runs before any frontend checkout.
steps = dict(re.findall(r"^      - name: ([^\n]+)\n([\s\S]*?)(?=^      - |\Z)", TEXT, re.MULTILINE))


def step(name: str) -> str:
    if name not in steps:
        fail(f"missing workflow step: {name}")
    return steps[name]


def unconditional(name: str) -> str:
    body = step(name)
    if re.search(r"^        (?:if|continue-on-error):", body, re.MULTILINE):
        fail(f"{name} must run unconditionally and fail on errors")
    return body


for name, command in {
    "Generate Swagger documents": "./script/generate-swagger.sh",
    "Verify generated document is committed": "git diff --exit-code -- ppanel.json",
    "Verify Swagger covers Hertz routes": "go test ./internal/transport/http/routes -run '^TestSwagger' -count=1",
    "Verify Swagger workflow contract": "python3 script/check-swagger-workflow-contract.py",
}.items():
    if f"run: {command}" not in unconditional(name):
        fail(f"{name} must retain its verification command")

artifact = unconditional("Retain generated Swagger documents")
for pattern in (
    r"uses:\s*actions/upload-artifact@",
    r"path:\s*build/swagger/\*\.json\s*$",
    r"if-no-files-found:\s*error\s*$",
):
    if not re.search(pattern, artifact, re.MULTILINE):
        fail("verified Swagger JSONs must be retained as an artifact, including on forks")

# Only the actual upstream repository's non-PR master context may sync upstream.
# Every cross-repository step needs the complete guard: a guarded checkout alone
# would still leave copy/commit/push commands running in a fork.
sync_guard = "github.repository == 'perfect-panel/backend' && github.event_name != 'pull_request' && github.ref == 'refs/heads/master'"
for name in ("Checkout frontend repository", "Copy Swagger documents", "Commit frontend Swagger changes"):
    guard = re.search(r"^        if:\s*([^\n]+)$", step(name), re.MULTILINE)
    if not guard or guard.group(1).strip() != sync_guard:
        fail(f"{name} must be gated to actual upstream non-PR master runs")

# The target repository is frontend main checked out under ./frontend.
require(r"repository:\s*perfect-panel/frontend\b", "workflow must checkout perfect-panel/frontend")
require(r"ref:\s*main\b", "frontend checkout must pin ref: main")
require(r"path:\s*frontend\b", "frontend checkout must use path: frontend")
require(r"frontend/docs/public/swagger", "workflow must sync into frontend/docs/public/swagger")

# Sync every generated top-level JSON dynamically instead of maintaining a stale manual list.
require(r"manifest=['\"]frontend/docs/public/swagger/\.backend-generated['\"]", "workflow must track backend-owned files in a manifest")
require(r"done\s*<\s*['\"]?\$\{manifest\}['\"]?", "workflow must remove files listed by the previous backend manifest")
require(r"find\s+build/swagger\b[^\n]*-maxdepth\s+1[^\n]*-name\s+['\"]\*\.json['\"][^\n]*-printf\s+['\"]%f", "workflow must discover all top-level build/swagger/*.json files dynamically")
require(r"cp\s+['\"]build/swagger/\$\{file\}['\"]\s+frontend/docs/public/swagger/", "workflow must copy each discovered JSON into frontend/docs/public/swagger")

# Only generated Swagger JSON should be committed in the frontend repository, and no-op syncs should not commit.
require(r"working-directory:\s*frontend\b", "commit step must run in frontend checkout")
require(r"git add\s+--all\s+--\s+docs/public/swagger", "commit step must stage additions, updates, and removals under docs/public/swagger")
require(r"git diff --cached --quiet", "workflow must skip commit when there are no changes")
require(r"git commit -m ['\"]docs\(api\): sync Swagger from backend['\"]", "workflow must use the required Swagger sync commit message")

# Old docs repo and frontend client generation must not be touched by this workflow.
if "perfect-panel/ppanel-docs" in TEXT or "ppanel-docs" in TEXT:
    fail("workflow must not checkout or write perfect-panel/ppanel-docs")
if re.search(r"openapi2ts", TEXT, re.IGNORECASE):
    fail("workflow must not run openapi2ts")
print("Swagger workflow contract OK")
