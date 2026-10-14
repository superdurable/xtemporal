#!/usr/bin/env python3
import re
import statistics
import subprocess
import sys

NAMESPACE = "md5('0')"
ORDER_PG = "ORDER BY coalesce(close_time, '9999-12-31 23:59:59') DESC, start_time DESC, run_id LIMIT 20"
ORDER_PDB = 'ORDER BY sort_close_time DESC, start_time DESC, run_id COLLATE "C" LIMIT 20'
PREDICATES = {
    "status_time": "status = 1 AND start_time >= '2026-01-01' AND start_time < '2026-01-02'",
    "type_status": "workflow_type_name = 'type-0' AND status = 1",
    "text": "Text01 {text_operator} 'hello'",
    "keyword_in": "Keyword01 IN ('tenant-0', 'tenant-20')",
    "keyword_list": "{list_predicate}",
}


def benchmark(container, paradedb):
    anchor = f"namespace_id {'@@@' if paradedb else '='} {NAMESPACE}"
    queries = {}
    for name, predicate in PREDICATES.items():
        predicate = predicate.format(text_operator='|||' if paradedb else '@@', list_predicate="KeywordList01 @@@ pdb.term('tag-0')" if paradedb else "KeywordList01 @> '[\"tag-0\"]'::jsonb")
        queries[name] = f"SELECT run_id FROM executions_visibility WHERE {anchor} AND {predicate} {ORDER_PDB if paradedb else ORDER_PG}"
    script = ''.join(f"\\echo RUNCASE {name}\n" + ''.join(f"EXPLAIN (ANALYZE) {query};\n" for _ in range(20)) for name, query in queries.items())
    result = subprocess.run(
        ["docker", "exec", "-i", container, "psql", "-U", "postgres", "-d", "visibility", "-v", "ON_ERROR_STOP=1"],
        input=script, text=True, capture_output=True, check=True,
    )
    for name in queries:
        section = result.stdout.split(f"RUNCASE {name}\n", 1)[1].split("RUNCASE ", 1)[0]
        timings = [float(t) for t in re.findall(r"Execution Time: ([0-9.]+) ms", section)]
        if len(timings) != 20:
            raise RuntimeError(f"{name}: got {len(timings)} plans")
        timings.sort()
        print(f"{container} {name}: p50={statistics.median(timings):.3f} ms p95={timings[18]:.3f} ms")


if __name__ == "__main__":
    if len(sys.argv) == 1 or sys.argv[1] == "paradedb":
        benchmark("xtpdbspike-paradedb-1", True)
    if len(sys.argv) == 1 or sys.argv[1] == "postgres":
        benchmark("xtpdbspike-postgres-1", False)
