"""Review or restore archived SubGate files without overwriting later edits."""
import argparse
import hashlib
import json
from pathlib import Path


def digest(data):
    # Git may normalize CRLF in the shared Windows/Linux worktree.
    return hashlib.sha256(data.replace(b"\r\n", b"\n")).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--apply", action="store_true", help="restore verified files")
    args = parser.parse_args()
    archive = Path(__file__).resolve().parent
    root = archive.parent.parent
    entries = json.loads((archive / "manifest.json").read_text(encoding="utf-8"))
    pending = []
    for entry in entries:
        source = (archive / entry["archive"]).resolve()
        target = (root / entry["path"]).resolve()
        if not source.is_relative_to(archive / "originals") or not target.is_relative_to(root):
            raise SystemExit("Manifest path outside archive/workspace")
        data = source.read_bytes()
        if hashlib.sha256(data).hexdigest() != entry["original_sha256"]:
            raise SystemExit("Archive checksum mismatch: " + entry["path"])
        current = target.read_bytes() if target.exists() else None
        if current is not None and digest(current) == digest(data):
            continue
        expected = entry["inactive_normalized_sha256"]
        if (current is None and expected is not None) or (
            current is not None and digest(current) != expected
        ):
            raise SystemExit("Later edits detected; compare manually: " + entry["path"])
        pending.append((target, data))
        print(entry["path"])
    if args.apply:
        for target, data in pending:
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(data)
        print(f"Restored {len(pending)} files; archive retained")
    else:
        print(f"Verified {len(entries)} archive entries; {len(pending)} files to restore")
        print("Review the list, then use --apply to restore")


if __name__ == "__main__":
    main()
