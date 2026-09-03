#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
补丁：处理「三元表达式 / 模板字符串」里的硬编码色值。

tokenize_styles.py 只匹配 `prop: 'value'`（整值单色），因此
`background: cond ? 'rgba(255,255,255,0.06)' : 'var(--x)'` 这类会漏。

本脚本按**行**处理：对每个颜色字面量，向前找同一行最近的 `prop:` 作为语义上下文，
复用 tokenize_styles 的映射表。能覆盖绝大多数单行三元写法。

用法：
  python3 fix_ternary.py           # dry-run
  python3 fix_ternary.py --apply   # 写入
"""
import os
import re
import sys
import collections

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from tokenize_styles import map_color, BG_PROPS, BORDER_PROPS, TEXT_PROPS  # noqa: E402

ROOT = os.path.abspath(
    os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', '..', 'frontend-v2', 'src')
)
SKIP_DIRS = {'office'}
APPLY = '--apply' in sys.argv

# 颜色字面量（带引号）
COL = re.compile(r"'(?P<c>rgba\(255,\s*255,\s*255,\s*0\.\d+\)|#[0-9A-Fa-f]{6})'")
# 属性名
PROP = re.compile(r"(?P<p>[A-Za-z][A-Za-z0-9]*)\s*:")

stats = collections.Counter()
missed = collections.Counter()
changed = []

for dp, _dn, fns in os.walk(ROOT):
    for fn in fns:
        if not fn.endswith('.tsx'):
            continue
        path = os.path.join(dp, fn)
        rel = os.path.relpath(path, ROOT).replace('\\', '/')
        if rel.split('/')[0] in SKIP_DIRS:
            continue

        with open(path, encoding='utf-8') as f:
            lines = f.read().split('\n')

        dirty = False
        for i, line in enumerate(lines):
            if not COL.search(line):
                continue
            # 该行所有属性的位置
            props = [(m.start(), m.group('p')) for m in PROP.finditer(line)]
            if not props:
                continue

            def replace(m):
                pos = m.start()
                # 向前找最近的 prop:
                owner = None
                for ppos, pname in reversed(props):
                    if ppos < pos:
                        owner = pname
                        break
                if owner is None:
                    return m.group(0)
                # backdropFilter 的特殊值（blur(... )）不在此处理
                new, table = map_color(owner, m.group('c'))
                if new:
                    stats[table] += 1
                    return f"'{new}'"
                missed[(owner, m.group('c'))] += 1
                return m.group(0)

            new_line = COL.sub(replace, line)
            if new_line != line:
                lines[i] = new_line
                dirty = True

        if dirty:
            changed.append(rel)
            if APPLY:
                with open(path, 'w', encoding='utf-8', newline='') as f:
                    f.write('\n'.join(lines))

print(f'=== {"已写入" if APPLY else "DRY-RUN（未写入）"} ===')
print(f'改动文件数：{len(changed)}')
for k, v in stats.most_common():
    print(f'  {k:8s} {v}')
print(f'  合计     {sum(stats.values())}')

if missed:
    print(f'\n=== 仍无法映射：{sum(missed.values())} 处 ===')
    for (p, c), n in missed.most_common(20):
        print(f'  {n:3d}  {p}: {c}')
