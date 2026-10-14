#!/usr/bin/env python3
"""Adjust wire lengths for rewritten go_package strings in generated Go files."""

import pathlib
import re
import sys

PREFIX = "github.com/superdurable/xtemporal"
PATTERN = re.compile(
    r"B(?P<outer>.)Z(?P<inner>.)"
    + re.escape(PREFIX)
    + r'(?P<tail>[^"\\]*)'
)


def fix(match):
    outer = ord(match["outer"])
    inner = ord(match["inner"])
    tail = match["tail"]
    if not tail.endswith("b"):
        raise ValueError("unexpected protobuf go_package suffix")
    actual = len(PREFIX + tail[:-1])
    if inner == actual and outer == actual + 2:
        return match.group()
    if inner != actual - len(PREFIX) + len("go.temporal.io/server") or outer != inner + 2:
        raise ValueError("unexpected protobuf descriptor length")
    if actual + 2 >= 128:
        raise ValueError("protobuf descriptor length needs multi-byte encoding")
    return "B" + chr(actual + 2) + "Z" + chr(actual) + PREFIX + tail


root = pathlib.Path(sys.argv[1])
for path in root.rglob("*.pb.go"):
    source = path.read_text()
    result = PATTERN.sub(fix, source)
    if result != source:
        path.write_text(result)
