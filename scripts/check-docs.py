#!/usr/bin/env python3
"""Validate documentation links, reorg preservation and navigation without reading secrets."""
from __future__ import annotations
import hashlib
import json
import re
import sys
import zipfile
from pathlib import Path
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[1]
ARCHIVE = ROOT / "docs/_archive/reorg-2026-10-17"
LINK = re.compile(r"(?<!!)\[([^\]\n]+)\]\(([^)\n]+)\)")
FENCE = re.compile(r"(^[ \t]*(\x60{3,}|~{3,}).*$)[\s\S]*?^[ \t]*\2[ \t]*$", re.M)


def prose(path: Path) -> str:
    return FENCE.sub("", path.read_text())


def anchors(path: Path) -> set[str]:
    found: set[str] = set()
    duplicates: dict[str, int] = {}
    for heading in re.findall(r"^#{1,6}\s+(.+?)\s*#*$", prose(path), re.M):
        heading = re.sub(r"<[^>]+>", "", heading).lower()
        heading = re.sub(r"[^\w\- ]", "", heading, flags=re.UNICODE).replace(" ", "-")
        n = duplicates.get(heading, 0)
        duplicates[heading] = n + 1
        found.add(heading if not n else f"{heading}-{n}")
    found.update(re.findall(r'<(?:a|h[1-6])\b[^>]*\bid=["\']([^"\']+)', path.read_text()))
    return found


def link_target(path: Path, raw: str) -> tuple[Path, str] | None:
    raw = raw.strip().split(' "', 1)[0].strip("<>")
    parsed = urlsplit(raw)
    if parsed.scheme or parsed.netloc:
        return None
    return ((path.parent / unquote(parsed.path)).resolve() if parsed.path else path, unquote(parsed.fragment))


def check() -> list[str]:
    errors: list[str] = []
    files = sorted(set(ROOT.glob("*.md")) | set((ROOT / "docs").rglob("*.md")))
    for path in files:
        for _, raw in LINK.findall(prose(path)):
            target = link_target(path, raw)
            if target is None:
                continue
            destination, fragment = target
            if not destination.exists():
                errors.append(f"{path.relative_to(ROOT)}: missing {raw}")
            elif fragment and destination.suffix == ".md" and fragment not in anchors(destination):
                errors.append(f"{path.relative_to(ROOT)}: missing anchor {raw}")

    manifest = json.loads((ARCHIVE / "manifest.json").read_text())
    with zipfile.ZipFile(ARCHIVE / "original-markdown.zip") as original:
        for entry in manifest:
            data = original.read(entry["path"])
            if len(data) != entry["bytes"] or hashlib.sha256(data).hexdigest() != entry["sha256"]:
                errors.append(f"original snapshot differs: {entry['path']}")
        for name in ["subgate-ai-prompt-v4.md", "subgate-feature-inventory.md"]:
            if (ROOT / name).read_bytes() != original.read(name):
                errors.append(f"protected requirements changed: {name}")

    index = ROOT / "docs/INDEX.md"
    if len(index.read_text().splitlines()) > 80:
        errors.append("INDEX exceeds 80 lines")
    if len((ROOT / "AGENTS.md").read_text().splitlines()) > 80:
        errors.append("AGENTS exceeds 80 lines")
    domains = sorted(p for p in (ROOT / "docs/registry").glob("*.md"))
    if not 10 <= len(domains) <= 12:
        errors.append(f"expected 10..12 domain files, got {len(domains)}")
    for path in domains:
        if not 3000 <= path.stat().st_size <= 12 * 1024:
            errors.append(f"domain size outside 3..12 KiB: {path.name}")

    active = [p for p in (ROOT / "docs").rglob("*") if p.is_file() and "_archive" not in p.parts]
    baseline = sum(e["bytes"] for e in manifest if e["path"].startswith("docs/"))
    if sum(p.stat().st_size for p in active) >= baseline:
        errors.append("active docs did not shrink")
    for path in active:
        if path.suffix == ".md" and path != index and str(path.relative_to(index.parent)) not in index.read_text():
            errors.append(f"active document absent from INDEX: {path.relative_to(ROOT)}")

    features = re.findall(r"^\| (\d+) / F-\d{3} \|", (ROOT / "docs/FEATURE_STATUS.md").read_text(), re.M)
    if sorted(map(int, features)) != list(range(1, 37)):
        errors.append("FEATURE_STATUS must cover each Part B row 1..36 exactly once")
    references = re.findall(r"^\| F-(\d+) / (\d+) \|", (ROOT / "docs/FEATURE_STATUS.md").read_text(), re.M)
    source_lines = list(range(15, 28)) + list(range(30, 41)) + [43, 46, 49] + list(range(52, 57)) + [59, 62, 65, 68, 71]
    if sorted((int(ident), int(line)) for ident, line in references) != list(zip(range(214, 251), source_lines)):
        errors.append("FEATURE_STATUS must preserve all 37 Part A reference IDs and source lines")
    skills = ROOT / ".claude/skills"
    if not skills.is_symlink() or skills.resolve() != (ROOT / ".agents/skills").resolve():
        errors.append("Claude skills must link to canonical .agents/skills")
    source = (ROOT / "docs/PROJECT_PLAN.md").read_text()
    tasks = re.findall(r"^- \[([ x!])\] (\d+\.\d+) (.+)$", source, re.M)
    if not tasks or len({t[1] for t in tasks}) != len(tasks):
        errors.append("task IDs absent or duplicated")
    for status, task, content in tasks:
        expected = "проверено:" if status == "x" else "блокер:" if status == "!" else "критерий готовности:"
        if expected not in content:
            errors.append(f"task lacks verifiable criterion: {task}")

    samples = [(1, "gateway-routing.md"), (5, "subscription.md"), (16, "activity.md"),
               (21, "ip-origins.md"), (27, "transports.md")]
    if "docs/INDEX.md" not in (ROOT / "AGENTS.md").read_text():
        errors.append("AGENTS does not point to INDEX")
    for number, domain in samples:
        relative = f"registry/{domain}"
        if relative not in index.read_text() or not (index.parent / relative).is_file():
            errors.append(f"two-open navigation failed for Part B #{number}")
    return errors


if __name__ == "__main__":
    failures = check()
    if failures:
        print(f"FAIL: {len(failures)} documentation checks")
        print("\n".join(failures[:20]))
        sys.exit(1)
    print("PASS: links/anchors, 60 originals SHA256, protected sources, sizes, 36 features, 37 references, skills symlink, task IDs, 5/5 two-open routes")
