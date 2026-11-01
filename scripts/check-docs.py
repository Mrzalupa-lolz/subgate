#!/usr/bin/env python3
"""Check current root/docs Markdown links and navigation; never scan middleware."""
from __future__ import annotations

import re
import sys
from pathlib import Path
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[1]
LINK = re.compile(r"!?\[[^\]\n]*\]\((<[^>\n]+>|[^)\n]+)\)")
REFERENCE = re.compile(r'^ {0,3}\[[^\]\n]+\]:\s*(<[^>]+>|\S+)', re.M)
FENCE = re.compile(r"^[ \t]*(`{3,}|~{3,})")


def prose(path: Path) -> str:
    """Exclude fenced code so examples are not mistaken for links/headings."""
    lines: list[str] = []
    fence_char = ""
    fence_length = 0
    for line in path.read_text(encoding="utf-8").splitlines():
        match = FENCE.match(line)
        if match:
            marker = match[1]
            if not fence_char:
                fence_char, fence_length = marker[0], len(marker)
            elif marker[0] == fence_char and len(marker) >= fence_length and not line[match.end():].strip():
                fence_char = ""
            continue
        if not fence_char:
            lines.append(line)
    return "\n".join(lines)


def anchors(path: Path) -> set[str]:
    source = prose(path)
    found: set[str] = set()
    for heading in re.findall(r"^ {0,3}#{1,6}\s+(.+?)\s*#*$", source, re.M):
        heading = re.sub(r"<[^>]+>", "", heading).lower()
        heading = re.sub(r"[^\w\- ]", "", heading).replace(" ", "-")
        candidate = heading
        number = 0
        while candidate in found:
            number += 1
            candidate = f"{heading}-{number}"
        found.add(candidate)
    found.update(re.findall(r'''<(?:a|h[1-6])\b[^>]*\bid=["']([^"']+)''', source))
    return found


def link_target(path: Path, raw: str) -> tuple[Path, str] | None:
    raw = raw.strip()
    if raw.startswith("<"):
        raw = raw[1:raw.index(">")]
    else:
        raw = raw.split(maxsplit=1)[0]
    parsed = urlsplit(raw)
    if parsed.scheme or parsed.netloc:
        return None
    destination = (path.parent / unquote(parsed.path)).resolve() if parsed.path else path.resolve()
    return destination, unquote(parsed.fragment)


def check(root: Path = ROOT) -> list[str]:
    root = root.resolve()
    docs = root / "docs"
    files = sorted(set(root.glob("*.md")) | set(docs.rglob("*.md")))
    errors: list[str] = []
    targets: dict[Path, set[Path]] = {}
    for path in files:
        source = prose(path)
        targets[path] = set()
        for raw in LINK.findall(source) + REFERENCE.findall(source):
            target = link_target(path, raw)
            if target is None:
                continue
            destination, fragment = target
            targets[path].add(destination)
            # The middleware is an independent project; do not inspect it.
            if destination.is_relative_to(root / "remnawave-subscription-middleware"):
                continue
            if not destination.exists():
                errors.append(f"{path.relative_to(root)}: missing {raw}")
            elif fragment and destination.is_file() and destination.suffix == ".md" and fragment not in anchors(destination):
                errors.append(f"{path.relative_to(root)}: missing anchor {raw}")

    index = docs / "INDEX.md"
    readme = root / "README.md"
    if index not in targets:
        errors.append("missing docs/INDEX.md")
    else:
        for path in files:
            if path.is_relative_to(docs) and path != index and path not in targets[index]:
                errors.append(f"document absent from INDEX: {path.relative_to(root)}")
    if readme not in targets:
        errors.append("missing README.md")
    elif index not in targets[readme]:
        errors.append("README does not link to docs/INDEX.md")
    return errors


if __name__ == "__main__":
    failures = check()
    if failures:
        print(f"FAIL: {len(failures)} documentation checks")
        print("\n".join(failures))
        sys.exit(1)
    print("PASS: local links, images, anchors and documentation index")
