#!/usr/bin/env python3
"""Summarize cmd/perftrace JSONL without external Python dependencies."""

import argparse
import json
import math
from pathlib import Path
from statistics import median


def percentile(values, p):
    ordered = sorted(values)
    return ordered[max(0, math.ceil(p * len(ordered)) - 1)]


def times(values):
    if not values:
        return "no samples"
    milliseconds = [value / 1_000_000 for value in values]
    return (
        f"p50={median(milliseconds):.2f} ms  "
        f"p95={percentile(milliseconds, 0.95):.2f} ms  "
        f"max={max(milliseconds):.2f} ms"
    )


def summarize(path):
    manifest = None
    frames = []
    actions = []
    summary = None
    for number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        if not line.strip():
            continue
        try:
            record = json.loads(line)
        except json.JSONDecodeError as exc:
            raise ValueError(f"{path}:{number}: {exc}") from exc
        kind = record.get("Kind", record.get("kind"))
        if kind == "manifest":
            manifest = record
        elif kind == "frame":
            frames.append(record["Sample"])
        elif kind == "action":
            actions.append(record["DurationNS"])
        elif kind == "summary":
            summary = record
    print(f"{path}: {manifest.get('Scenario', 'unknown') if manifest else 'missing manifest'}")
    print(f"  frames={len(frames)} actions={len(actions)}")
    for phase in ("Build", "Layout", "Paint", "TextLayout", "Raster", "Upload", "Present"):
        print(f"  {phase:10} {times([frame[phase] for frame in frames])}")
    print(f"  {'action':10} {times(actions)}")
    if summary is None:
        print("  incomplete: missing summary record")
    else:
        print(
            "  dropped frames/actions, skipped actions: "
            f"{summary.get('dropped_frames', 0)}/"
            f"{summary.get('dropped_actions', 0)}/"
            f"{summary.get('skipped_actions', 0)}"
        )


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("files", nargs="+", type=Path)
    arguments = parser.parse_args()
    for file in arguments.files:
        summarize(file)
