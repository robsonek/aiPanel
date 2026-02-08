#!/usr/bin/env python3
"""Validate runtime lock fingerprints against detached signature issuers.

Usage:
  python3 scripts/check_runtime_signatures.py [/path/to/lock.json]
"""

from __future__ import annotations

import json
import re
import subprocess
import sys
import tempfile
import urllib.request
from pathlib import Path


FPR_RE = re.compile(r"([A-F0-9]{40})", re.IGNORECASE)


def extract_signature_fingerprint(signature_url: str) -> str:
    with tempfile.NamedTemporaryFile(prefix="aipanel-sig-", suffix=".asc", delete=True) as tf:
        with urllib.request.urlopen(signature_url, timeout=120) as response:
            tf.write(response.read())
            tf.flush()
        out = subprocess.check_output(
            ["gpg", "--batch", "--list-packets", tf.name],
            text=True,
            stderr=subprocess.STDOUT,
        )
    for line in out.splitlines():
        if "issuer fpr v4" not in line:
            continue
        match = FPR_RE.search(line.upper())
        if match:
            return match.group(1).upper()
    return ""


def main() -> int:
    lock_path = Path(sys.argv[1] if len(sys.argv) > 1 else "configs/sources/lock.json")
    with lock_path.open("r", encoding="utf-8") as fh:
        lock = json.load(fh)

    mismatches = 0
    channels = lock.get("channels", {})
    for channel_name, channel in channels.items():
        print(f"[{channel_name}]")
        for component_name, component in sorted(channel.items()):
            signature_url = str(component.get("signature_url", "")).strip()
            expected = str(component.get("public_key_fingerprint", "")).strip().upper()
            if not signature_url:
                print(f"  {component_name}: missing signature_url")
                mismatches += 1
                continue
            try:
                actual = extract_signature_fingerprint(signature_url)
            except Exception as exc:  # noqa: BLE001
                print(f"  {component_name}: error reading signature ({exc})")
                mismatches += 1
                continue
            status = "OK" if actual == expected else "MISMATCH"
            print(
                f"  {component_name}: expected={expected} signature={actual} status={status}",
            )
            if status != "OK":
                mismatches += 1

    if mismatches:
        print(f"\nFound {mismatches} mismatch(es).")
        return 1
    print("\nAll runtime signature fingerprints match lock file.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
