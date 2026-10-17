#!/usr/bin/env python3
"""Render a dex-compare.sh output directory as Markdown tables (summary.md)."""
import json
import pathlib
import re
import sys

SHAPES = ('main_list', 'need_person', 'done_today', 'running', 'custom_eq', 'custom_range', 'workflow_id',
          'readyz', 'schedules', 'text_bm25', 'custom_only_bm25')
PHASES = (('mix-no-c3-16', 'no C3'), ('mix-c3-16', 'C3'), ('mix-no-c3-64', 'no C3'), ('mix-c3-64', 'C3'))
MIB = 1048576


def load(root, name):
    path = root / name
    return json.loads(path.read_text()) if path.exists() else None


def fmt(v, digits=1):
    if v is None or v == '':
        return '—'
    if isinstance(v, float):
        return f'{v:.{digits}f}'
    return str(v)


def table(header, rows):
    out = ['| ' + ' | '.join(header) + ' |', '|' + '|'.join(['---'] + ['---:'] * (len(header) - 1)) + '|']
    out += ['| ' + ' | '.join(fmt(c) for c in r) + ' |' for r in rows]
    return '\n'.join(out) + '\n'


def plan_path(plan):
    """Classify a text EXPLAIN into the access path that served the query."""
    if 'ParadeDB' in plan:
        method = re.search(r'Exec Method: (\w+)', plan)
        return 'ParadeDB ' + (method.group(1) if method else 'scan')
    scans = re.findall(r'(Index Only Scan|Index Scan|Bitmap Index Scan)(?: Backward)? (?:using|on) (\w+)', plan)
    if not scans:
        return 'Seq Scan' if 'Seq Scan' in plan else plan.splitlines()[0]
    kind, index = scans[0]
    if kind == 'Bitmap Index Scan':
        path = f'{index} (bitmap + top-N sort)' if 'Sort Key' in plan else f'{index} (bitmap)'
    else:
        path = f'{index} (ordered scan)' if 'Sort Key' not in plan else f'{index} (scan + sort)'
    if re.search(r'^\s+Filter:', plan, re.M):
        path += ' + heap filter'
    if '@@@' in plan:
        path += ' [@@@!]'
    return path


def exec_ms(plan):
    m = re.search(r'Execution Time: ([\d.]+) ms', plan)
    return float(m.group(1)) if m else None


VARIANTS = (('no-c3', 'no C3'), ('c3', 'C3'), ('c3-custom', 'C3, custom plans'))


def mix_section(root):
    lines = []
    for writers in (16, 64):
        data = [(label, load(root, f'mix-{key}-{writers}.json')) for key, label in VARIANTS]
        data = [(label, d) for label, d in data if d]
        if not data:
            continue
        lines += [f'### {writers} writers', '']
        header = ['Shape'] + [f'{label} {m}' for label, _ in data for m in ('QPS', 'p50', 'p95', 'p99')]
        rows = []
        for s in SHAPES:
            row = [s]
            for _, d in data:
                r = d.get('reads', {}).get(s, {})
                row += [r.get('per_second'), r.get('p50_ms'), r.get('p95_ms'), r.get('p99_ms')]
            rows.append(row)
        lines.append(table(header, rows))
        wrows = []
        for label, d in data:
            wrows.append([label, d['write_statements_per_second'], d['polls'], d['late_polls']])
        lines.append(table(['Variant', 'write statements/s', 'polls', 'late polls'], wrows))
        ops = sorted({op for _, d in data for op in d['writes']})
        orows = []
        for op in ops:
            row = [op]
            for _, d in data:
                r = d.get('writes', {}).get(op, {})
                row += [r.get('per_second'), r.get('p50_ms'), r.get('p99_ms')]
            orows.append(row)
        lines.append(table(['Write op'] + [f'{label} {m}' for label, _ in data for m in ('/s', 'p50', 'p99')], orows))
    return lines


def explain_section(root):
    lines = []
    for name, label in (('explain-no-c3.json', 'without C3'), ('explain-c3.json', 'with C3')):
        d = load(root, name)
        if not d:
            continue
        lines += [f'### Plans {label}', '']
        rows = []
        for key in sorted(d, key=lambda k: (SHAPES.index(k.split('/')[0]) if k.split('/')[0] in SHAPES else 99, k)):
            e = d[key]
            rows.append([key, e['intended'], plan_path(e['custom']), exec_ms(e['custom']),
                         plan_path(e['generic']) if 'generic' in e else '—', exec_ms(e['generic']) if 'generic' in e else None])
        lines.append(table(['Shape / namespace', 'Intended', 'Custom plan', 'ms', 'Generic plan', 'ms'], rows))
    return lines


def worst_section(root):
    lines = []
    for name, label in (('worst-idle.json', 'Idle, no writers, BM25 merged (29 segments)'), ('worst.json', 'Under writers, first run (right after the mixed phases)'), ('worst-rerun.json', 'Under writers, rerun')):
        lines += worst_one(root, name, label)
    return lines


def worst_one(root, name, label):
    d = load(root, name)
    if not d:
        return []
    lines = [f'### {label}', '',
             'Flow rows and rare matches: ' + ', '.join(f'{k} = {d[k]}' for k in sorted(d) if k.endswith(('_flow_rows', '_rare_matches'))) + '.',
             f"Promoted index: {d.get('promoted_index_bytes', 0) / MIB:.0f} MiB, built in {d.get('promoted_index_build_seconds', 0):.1f} s.", '']
    phases = [k for k in d if k == 'quiet' or k.startswith('under_')]
    keys = sorted({k for p in phases for k in d[p]})
    rows = []
    for k in keys:
        row = [k]
        for p in phases:
            r = d[p].get(k, {})
            row += [r.get('p50_ms'), r.get('p99_ms'), r.get('rows'), r.get('fallbacks', '')]
        rows.append(row)
    header = ['Variant / namespace / value']
    for p in phases:
        header += [f'{p} p50', f'{p} p99', 'rows', 'fallbacks']
    lines.append(table(header, rows))
    plans = d.get('plans', {})
    prow = [[k, plan_path(v), exec_ms(v)] for k, v in sorted(plans.items())]
    lines.append(table(['Plan', 'Path', 'ms'], prow))
    return lines


def paging_section(root):
    d = load(root, 'paging.json')
    if not d:
        return []
    r = d['result']
    rows = [[k, v] for k, v in (
        ('namespace', d['namespace']), ('filter', f"flow_type = {d['flow']} AND keyword1 = {d['keyword1']}"),
        ('set before / after', f"{d['set_before']} / {d['set_after']}"), ('pages', r['Pages']),
        ('paging seconds', d['paging_seconds']), ('stable rows', r['Stable']),
        ('stable duplicated', r['StableDuplicated']), ('stable skipped', r['StableSkipped']),
        ('open to closed', r['OpenToClosed']), ('open to closed, times seen', json.dumps(r['OpenToClosedSeen'])),
        ('moved otherwise', r['OtherMoved']), ('deleted during paging', r['Deleted']),
        ('inserted during paging (seen)', f"{r['Inserted']} ({r.get('InsertedSeen', 0)})"), ('passed', d['passed']))]
    wrows = [[op, v['count'], v['per_second'], v.get('p99_ms')] for op, v in sorted(d['writes'].items())]
    return [table(['Item', 'Value'], rows), table(['Concurrent write op', 'count', '/s', 'p99 ms'], wrows)]


def sizes_section(root):
    rows = []
    historical_tombstones = False
    for name in ('sizes-before.json', 'mix-no-c3-16-sizes.json', 'mix-c3-16-sizes.json', 'mix-c3-64-sizes.json',
                 'mix-no-c3-64-sizes.json', 'mix-c3-custom-16-sizes.json', 'mix-c3-custom-64-sizes.json', 'sizes-after.json'):
        d = load(root, name)
        if not d:
            continue
        idx = d['index_bytes']
        btree = sum(v for k, v in idx.items() if k != 'visibility_core_bm25')
        row = [name.replace('.json', ''), d['live_rows_estimate'], d['heap_bytes'] / MIB, btree / MIB,
               idx.get('visibility_core_bm25', 0) / MIB, d['bm25_segments']]
        if 'tombstone_rows' in d:
            historical_tombstones = True
        row += [d.get('tombstone_rows'), d.get('tombstone_bytes', 0) / MIB]
        rows.append(row)
    header = ['Snapshot', 'live rows', 'heap MiB', 'B-tree MiB', 'BM25 MiB', 'BM25 segments']
    if historical_tombstones:
        header += ['tombstones', 'tombstone MiB']
    else:
        rows = [row[:-2] for row in rows]
    lines = [table(header, rows)]
    d = load(root, 'sizes-before.json')
    if d:
        lines.append(table(['Index', 'MiB'], [[k, v / MIB] for k, v in sorted(d['index_bytes'].items())]))
    return lines


def main(root):
    lines = ['# Dex-profile benchmark summary', '']
    host = root / 'host.txt'
    if host.exists():
        lines += ['```', host.read_text().strip(), '```', '']
    ld = load(root, 'load.json')
    if ld:
        lines += [f"Loaded {ld['rows']:,} rows in {ld['copy_seconds']:.0f} s; vacuum and analyze {ld['vacuum_seconds']:.0f} s. "
                  'Index builds: ' + ', '.join(f'{k} {v:.0f} s' for k, v in sorted(ld['index_build_seconds'].items())) + '.']
    c3 = load(root, 'c3-build.json')
    if c3:
        lines += [f"`by_flow_type_status` built in {c3['build_seconds']:.0f} s, {c3['bytes'] / MIB:.0f} MiB.", '']
    for title, section in (('Read and write latency', mix_section), ('Plan evidence', explain_section),
                           ('Worst case', worst_section), ('Page-token stability', paging_section), ('Storage', sizes_section)):
        body = section(root)
        if body:
            lines += [f'## {title}', ''] + body + ['']
    (root / 'summary.md').write_text('\n'.join(lines).rstrip() + '\n')


if __name__ == '__main__':
    if len(sys.argv) != 2:
        raise SystemExit('usage: dex-summarize.py <output-dir>')
    main(pathlib.Path(sys.argv[1]))
