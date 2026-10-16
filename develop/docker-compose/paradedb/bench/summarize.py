#!/usr/bin/env python3
"""Render completed comparison runs as side-by-side Markdown tables.

Each variant directory holds one r<N> directory per round; numbers are medians across completed rounds.
"""
import json
import pathlib
import statistics
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
        numeric = [cell for cell in cells if isinstance(cell, (int, float))]
        best = (min if lower else max)(numeric) if numeric else None
        rendered = []
        for cell in cells:
            value = f'{cell:.2f}' if isinstance(cell, float) else str(cell)
            if isinstance(cell, (int, float)) and cell == best:
                value = f'**{value}**'
            rendered.append(value.replace('|', '\\|'))
        lines.append('| ' + label + ' | ' + ' | '.join(rendered) + ' |')
    return '\n'.join(lines) + '\n'


def median(values):
    values = [v for v in values if isinstance(v, (int, float))]
    return float(statistics.median(values)) if values else '—'


def rounds(root, variant):
    if (root / variant / 'done').exists():
        return [root / variant]  # single-round layout written before rounds existed
    return sorted((d for d in (root / variant).glob('r*') if (d / 'done').exists()), key=lambda d: int(d.name[1:]))


def load(dirs, name):
    return [json.loads((d / name).read_text()) for d in dirs if (d / name).exists()]


def spread(values):
    return ' / '.join(f'{v:.0f}' for v in values) if values else '—'


def main(root):
    done = {v: rounds(root, v) for v in VARIANTS}
    variants = [v for v in VARIANTS if done[v]]
    if not variants:
        raise SystemExit('no completed variants')
    levels = sorted({int(p.stem) for v in variants for d in done[v] for p in d.glob('*.json') if p.stem.isdigit()})
    lines = ['# Visibility benchmark comparison', '',
             'Numbers are medians across completed rounds; rounds per variant: ' +
             ', '.join(f'{v}={len(done[v])}' for v in variants) + '.',
             'Bold numbers are best among completed variants for that row; ties are all marked.',
             'Latency, errors and missed reads: lower is better. Throughput: higher is better.', '']

    capacity = {v: {q: load(done[v], f'capacity-{q}.json') for q in QUERIES} for v in variants}
    if any(capacity[v][q] for v in variants for q in QUERIES):
        lines += ['## Read capacity (read-only, closed loop, one query at a time)', '']
        lines.append(table('Reads/s', variants, [
            (q, [median([r['reads'][q]['per_second'] for r in capacity[v][q]]) for v in variants]) for q in QUERIES]))
        for metric in ('p50_ms', 'p99_ms'):
            lines.append(table(f'Read {metric}', variants, [
                (q, [median([r['reads'][q].get(metric) for r in capacity[v][q]]) for v in variants]) for q in QUERIES],
                lower=True))

    for level in levels:
        available = [v for v in variants if load(done[v], f'{level}.json')]
        data = {v: load(done[v], f'{level}.json') for v in available}
        rate = data[available[0]][0].get('read_rate', 0)
        lines += [f'## {level} writers', '']
        if rate:
            lines += [f'Readers offer a fixed {rate:g} reads/s per query ({rate * len(QUERIES):g}/s in total), '
                      'so every variant faces the same read pressure. Missed reads mean the variant could not keep up.', '']
        lines.append(table('Writes', available, [
            ('Writes/s', [median([r['write']['per_second'] for r in data[v]]) for v in available]),
            ('Writes/s per round', [spread([r['write']['per_second'] for r in data[v]]) for v in available]),
        ]))
        lines.append(table('Write latency and errors', available, [
            (metric, [median([r['write'].get(metric) for r in data[v]]) for v in available])
            for metric in ('p50_ms', 'p95_ms', 'p99_ms', 'errors')], lower=True))
        lines.append(table('Reads/s served', available, [
            (q, [median([r['reads'][q]['per_second'] for r in data[v]]) for v in available]) for q in QUERIES]))
        for metric in ('p50_ms', 'p95_ms', 'p99_ms', 'missed', 'errors'):
            lines.append(table(f'Read {metric}', available, [
                (q, [median([r['reads'][q].get(metric) for r in data[v]]) for v in available]) for q in QUERIES],
                lower=True))
        sizes = {v: [tuple(int(x) for x in (d / f'after-{level}-size.txt').read_text().strip().split('|'))
                     for d in done[v] if (d / f'after-{level}-size.txt').exists()] for v in available}
        lines.append(table('Storage after level', available, [
            ('Heap MiB', [median([s[0] / 1048576 for s in sizes[v]]) for v in available]),
            ('Index MiB', [median([s[1] / 1048576 for s in sizes[v]]) for v in available]),
        ], lower=True))
        lines += ['### Background load', '']
        for v in available:
            for d in done[v]:
                note = (d / f'{level}-background.txt').read_text().strip()
                host = (d / f'{level}-host-load.txt').read_text().strip()
                lines.append(f'- **{v} {d.name}:** {note} Host: `{host}`')
        lines.append('')

    checks = {v: json.loads((done[v][0] / 'check.json').read_text()) for v in variants}
    lines += ['## Query plans (before timing)', '',
              '| Query | ' + ' | '.join(variants) + ' |',
              '| --- | ' + ' | '.join(['---'] * len(variants)) + ' |']
    for query in QUERIES:
        lines.append('| ' + query + ' | ' + ' | '.join(plan_type(checks[v]['plans'][query]) for v in variants) + ' |')
    (root / 'summary.md').write_text('\n'.join(lines).rstrip() + '\n')


if __name__ == '__main__':
    if len(sys.argv) != 2:
        raise SystemExit('usage: summarize.py <output-dir>')
    main(pathlib.Path(sys.argv[1]))
