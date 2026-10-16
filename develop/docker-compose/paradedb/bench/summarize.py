#!/usr/bin/env python3
"""Render completed comparison runs as side-by-side Markdown tables."""
import json
import pathlib
import sys

VARIANTS = ('postgres', 'pdb-cols', 'pdb-cols-hybrid', 'pdb-json-hybrid')
QUERIES = ('status_time', 'type_status', 'text', 'keyword', 'keyword_list', 'count', 'group_count')


def plan_type(plan):
    for marker, label in (
        ('ParadeDB Top K Scan', 'ParadeDB Top K'),
        ('ParadeDB Aggregate Scan', 'ParadeDB Aggregate'),
        ('ParadeDB Base Scan', 'ParadeDB Base'),
        ('Bitmap Heap Scan', 'Bitmap heap/index'),
        ('Index Only Scan', 'B-tree index only'),
        ('Index Scan', 'B-tree index'),
        ('Seq Scan', 'Heap scan/filter'),
    ):
        if marker in plan:
            return label
    return plan.splitlines()[0].strip() if plan else 'unknown'


def table(title, variants, values, lower=False):
    lines = [f'### {title}', '', '| Metric | ' + ' | '.join(variants) + ' |',
             '| --- | ' + ' | '.join(['---:'] * len(variants)) + ' |']
    for label, cells in values:
        numeric = [(i, cell) for i, cell in enumerate(cells) if isinstance(cell, (int, float))]
        best = (min if lower else max)([v for _, v in numeric]) if numeric else None
        rendered = []
        for cell in cells:
            value = f'{cell:.2f}' if isinstance(cell, float) else str(cell)
            if isinstance(cell, (int, float)) and cell == best:
                value = f'**{value}**'
            rendered.append(value.replace('|', '\\|'))
        lines.append('| ' + label + ' | ' + ' | '.join(rendered) + ' |')
    return '\n'.join(lines) + '\n'


def main(root):
    variants = [v for v in VARIANTS if (root / v / 'done').exists()]
    if not variants:
        raise SystemExit('no completed variants')
    levels = sorted({int(p.stem) for v in variants for p in (root / v).glob('*.json') if p.stem.isdigit()})
    lines = ['# Visibility benchmark comparison', '',
             'Bold numbers are best among completed variants for that row; ties are all marked.',
             'Latency and errors: lower is better. Throughput: higher is better.', '']
    checks = {v: json.loads((root / v / 'check.json').read_text()) for v in variants}
    for level in levels:
        available = [v for v in variants if (root / v / f'{level}.json').exists()]
        data = {v: json.loads((root / v / f'{level}.json').read_text()) for v in available}
        lines += [f'## {level} writers', '']
        lines.append(table('Writes', available, [
            ('Writes/s', [data[v]['write']['per_second'] for v in available]),
        ]))
        lines.append(table('Write latency and errors', available, [
            (metric, [data[v]['write'].get(metric, '—') for v in available])
            for metric in ('p50_ms', 'p95_ms', 'p99_ms', 'errors')], lower=True))
        for query in QUERIES:
            lines.append(table(f'Read: {query}', available, [
                ('Reads/s', [data[v]['reads'][query]['per_second'] for v in available]),
            ]))
            lines.append(table(f'{query} latency and errors', available, [
                (metric, [data[v]['reads'][query].get(metric, '—') for v in available])
                for metric in ('p50_ms', 'p95_ms', 'p99_ms', 'errors')], lower=True))
        sizes = {}
        for v in available:
            heap, index = (root / v / f'after-{level}-size.txt').read_text().strip().split('|')
            sizes[v] = (int(heap), int(index))
        lines.append(table('Storage after level', available, [
            ('Heap MiB', [sizes[v][0] / 1048576 for v in available]),
            ('Index MiB', [sizes[v][1] / 1048576 for v in available]),
        ], lower=True))
        lines += ['### Query plans (before timing)', '',
                  '| Query | ' + ' | '.join(available) + ' |',
                  '| --- | ' + ' | '.join(['---'] * len(available)) + ' |']
        for query in QUERIES:
            lines.append('| ' + query + ' | ' + ' | '.join(plan_type(checks[v]['plans'][query]) for v in available) + ' |')
        lines += ['', '### Background load', '']
        for v in available:
            note = (root / v / f'{level}-background.txt').read_text().strip()
            host = (root / v / f'{level}-host-load.txt').read_text().strip()
            lines.append(f'- **{v}:** {note} Host: `{host}`')
        lines.append('')
    (root / 'summary.md').write_text('\n'.join(lines).rstrip() + '\n')


if __name__ == '__main__':
    if len(sys.argv) != 2:
        raise SystemExit('usage: summarize.py <output-dir>')
    main(pathlib.Path(sys.argv[1]))
