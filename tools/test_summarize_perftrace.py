"""Guard the validity checks used for paired performance comparisons."""

import contextlib
import io
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from summarize_perftrace import compare, summarize


SCRIPT = Path(__file__).with_name("summarize_perftrace.py")


class SummaryValidityTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)

    def trace(self, name, backend, tail=None):
        path = self.root / name
        records = [
            {"Kind": "manifest", "Scenario": "widgets", "Graphics": backend},
            {"Kind": "frame", "Sample": {"Backend": f"{backend}/test", "Build": 1_000_000}},
        ]
        if tail is None:
            records.append({"kind": "summary"})
        path.write_text("".join(json.dumps(record) + "\n" for record in records) + (tail or ""))
        return path

    def test_truncated_tail_rejects_speed_comparison(self):
        software = self.trace("software.jsonl", "software")
        hardware = self.trace("hardware.jsonl", "hardware", '{"Kind":"frame"')
        with contextlib.redirect_stdout(io.StringIO()):
            sw, hw = summarize(software), summarize(hardware)
        self.assertTrue(sw[2])
        self.assertFalse(hw[2])
        with self.assertRaisesRegex(ValueError, "incomplete trace"):
            compare(sw, hw)

    def test_invalid_trace_does_not_hide_later_valid_run(self):
        empty = self.root / "empty.jsonl"
        empty.touch()
        valid = self.trace("valid.jsonl", "software")
        process = subprocess.run(
            [sys.executable, str(SCRIPT), str(empty), str(valid)],
            capture_output=True, text=True, check=False,
        )
        self.assertEqual(process.returncode, 1)
        self.assertIn("empty trace", process.stdout)
        self.assertIn("frames=1 actions=0", process.stdout)
        self.assertNotIn("Traceback", process.stderr)


if __name__ == "__main__":
    unittest.main()
