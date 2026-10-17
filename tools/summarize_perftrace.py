#!/usr/bin/env python3
"""Summarize cmd/perftrace JSONL without external Python dependencies."""

import argparse
import json
import math
import sys
from collections import Counter
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


def frame_cost(frame):
    # Build includes Layout and Paint. CompletedGPU is asynchronous and
    # belongs to a previous frame; it must not be added to CPU submission.
    return sum(frame.get(key, 0) for key in ("Build", "Raster", "Upload", "Present", "GPUWait"))


def summarize(path):
    manifest = None
    frames = []
    actions = []
    summary = None
    lines = path.read_text(encoding="utf-8").splitlines(keepends=True)
    truncated = False
    for number, line in enumerate(lines, 1):
        if not line.strip():
            continue
        try:
            record = json.loads(line)
        except json.JSONDecodeError as exc:
            if number == len(lines) and not line.endswith("\n"):
                truncated = True
                break
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
    if manifest is None:
        raise ValueError(f"{path}: missing manifest ({'empty trace' if not lines else 'invalid trace'}; see matching .log)")
    print(f"{path}: {manifest.get('Scenario', 'unknown')} pattern={manifest.get('Pattern', 'unknown')} "
          f"requested={manifest.get('Graphics', 'unknown')} revision={manifest.get('Revision', 'unknown')}")
    print(f"  target={manifest.get('OS')}/{manifest.get('Arch')} "
          f"{manifest.get('CPU', '')} {manifest.get('GPU', '')} "
          f"power={manifest.get('PowerProfile', '')} display={manifest.get('Display', '')}")
    print(f"  frames={len(frames)} actions={len(actions)}")
    print(f"  backend={dict(Counter(frame.get('Backend', '') for frame in frames))}")
    reasons = Counter(frame.get("FallbackReason", "") for frame in frames if frame.get("FallbackReason"))
    if reasons:
        print(f"  fallback={dict(reasons)}")
    for phase in ("Build", "Layout", "Paint", "TextLayout", "Raster", "Upload", "Present", "GPUWait", "CompletedGPU"):
        values = [frame.get(phase, 0) for frame in frames]
        if phase == "CompletedGPU":
            values = [value for value in values if value > 0]
        print(f"  {phase:12} {times(values)}")
    costs = [frame_cost(frame) for frame in frames]
    print(f"  {'CPU total':12} {times(costs)}")
    print(f"  {'action':12} {times(actions)}")
    if frames:
        for budget in (16.7, 33.3, 50.0):
            print(f"  CPU frames > {budget:g} ms: {sum(value > budget * 1_000_000 for value in costs)}/{len(costs)}")
        print(f"  batches/frame p50={median(frame.get('Batches', 0) for frame in frames):g} "
              f"p95={percentile([frame.get('Batches', 0) for frame in frames], .95)}; "
              f"vertices/frame p95={percentile([frame.get('Vertices', 0) for frame in frames], .95)}")
        print(f"  texture uploads={sum(frame.get('TextureUploads', 0) for frame in frames)} "
              f"bytes={sum(frame.get('UploadBytes', 0) for frame in frames)}; "
              f"mask atlas fallbacks={sum(frame.get('MaskAtlasFallbacks', 0) for frame in frames)}")
    if truncated:
        print("  incomplete: truncated final JSON record")
    if summary is None:
        print("  incomplete: missing summary record")
    else:
        print(
            "  dropped frames/actions, skipped actions: "
            f"{summary.get('dropped_frames', 0)}/"
            f"{summary.get('dropped_actions', 0)}/"
            f"{summary.get('skipped_actions', 0)}"
        )

    return manifest, frames, summary is not None and not truncated


def compare(software, hardware):
    a, af, a_complete = software
    b, bf, b_complete = hardware
    if not a_complete or not b_complete:
        raise ValueError("comparison rejected: incomplete trace")
    keys = ("Scenario", "Pattern", "Count", "Width", "Height", "Interval", "Warmup", "OS", "Arch", "OSVersion", "Display", "CPU", "GPU", "PowerProfile", "Revision")
    differences = [key for key in keys if a.get(key) != b.get(key)]
    if differences:
        raise ValueError(f"comparison rejected: different {', '.join(differences)}")
    if not af or not bf:
        raise ValueError("comparison rejected: missing frames")
    if any(not frame.get("Backend", "").startswith("software/") for frame in af):
        raise ValueError("comparison rejected: software trace contains other backends")
    if any(not frame.get("Backend", "").startswith("hardware/") for frame in bf):
        raise ValueError("comparison rejected: hardware trace contains fallback or software frames")
    cpu_sw, cpu_hw = [frame_cost(frame) for frame in af], [frame_cost(frame) for frame in bf]
    if not median(cpu_sw) or not percentile(cpu_sw, .95):
        raise ValueError("comparison rejected: software durations are zero")
    print("CPU total ratio (hardware/software; smaller is better): "
          f"p50={median(cpu_hw) / median(cpu_sw):.2f} "
          f"p95={percentile(cpu_hw, .95) / percentile(cpu_sw, .95):.2f}")


def check_fallback(initialization, runtime):
    if not initialization[2] or not runtime[2]:
        raise ValueError("fallback check rejected: incomplete trace")
    init_frames, render_frames = initialization[1], runtime[1]
    if not init_frames or any(
        not frame.get("Backend", "").startswith("software/")
        or "injected Metal initialization" not in frame.get("FallbackReason", "")
        for frame in init_frames
    ):
        raise ValueError("initialization fault did not produce software frames with a matching fallback reason")
    kinds = [frame.get("Backend", "") for frame in render_frames]
    if "hardware/metal" not in kinds or not any(kind.startswith("software/") for kind in kinds):
        raise ValueError("runtime fault must record Metal frames followed by software frames")
    first_software = next(i for i, kind in enumerate(kinds) if kind.startswith("software/"))
    if any(kind.startswith("hardware/") for kind in kinds[first_software:]) or any(
        "injected Metal render failure" not in frame.get("FallbackReason", "")
        for frame in render_frames[first_software:]
    ):
        raise ValueError("runtime fallback is not persistent or lacks its failure reason")
    print(f"fallback verified: init={len(init_frames)} software frames, "
          f"runtime={first_software} Metal then {len(kinds)-first_software} software frames")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("files", nargs="+", type=Path)
    parser.add_argument("--compare", action="store_true", help="compare two files, software then hardware")
    parser.add_argument("--check-fallback", action="store_true", help="validate init and render fault traces")
    arguments = parser.parse_args()
    if (arguments.compare or arguments.check_fallback) and len(arguments.files) != 2:
        parser.error("--compare and --check-fallback each require two files")
    runs = []
    failed = False
    for file in arguments.files:
        try:
            run = summarize(file)
        except (OSError, ValueError) as exc:
            print(f"invalid trace: {exc}")
            run = None
        runs.append(run)
        failed |= run is None or not run[2]
    if arguments.compare:
        if None in runs:
            print("comparison rejected: invalid trace")
        else:
            try:
                compare(*runs)
            except ValueError as exc:
                print(exc)
                failed = True
    if arguments.check_fallback:
        if None in runs:
            print("fallback check rejected: invalid trace")
        else:
            try:
                check_fallback(*runs)
            except ValueError as exc:
                print(exc)
                failed = True
    sys.exit(1 if failed else 0)

