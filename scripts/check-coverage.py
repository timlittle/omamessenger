#!/usr/bin/env python3
"""Check Go statement coverage while excluding only the process entry point."""

from pathlib import Path
import re
import sys


def function_bounds(source: str, signature: str) -> tuple[int, int]:
    lines = source.splitlines()
    start = next(i for i, line in enumerate(lines) if re.match(signature, line))
    depth = 0
    for i in range(start, len(lines)):
        depth += lines[i].count("{") - lines[i].count("}")
        if depth == 0:
            return start + 1, i + 1
    raise ValueError("unterminated function")


def main() -> int:
    profile = Path(sys.argv[1])
    minimum = float(sys.argv[2])
    source_path = Path("backend/main.go")
    entry_start, entry_end = function_bounds(
        source_path.read_text(), r"func main\(\)\s*{"
    )

    total = covered = 0
    for record in profile.read_text().splitlines()[1:]:
        filename, data = record.split(":", 1)
        location, statements, count = data.rsplit(" ", 2)
        start, end = location.split(",")
        start_line = int(start.split(".")[0])
        end_line = int(end.split(".")[0])
        if filename.endswith("/backend/main.go") and start_line >= entry_start and end_line <= entry_end:
            continue
        statements, count = int(statements), int(count)
        total += statements
        if count:
            covered += statements

    percentage = 100 * covered / total if total else 0
    print(f"Core Go coverage (excluding process entry point): {percentage:.1f}% ({covered}/{total} statements)")
    if percentage < minimum:
        print(f"Coverage is below the required {minimum:.1f}%.", file=sys.stderr)
        return 1
    print(f"Coverage threshold passed ({minimum:.1f}%).")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
