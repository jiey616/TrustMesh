#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
圆角收敛：把内联样式里 12 种 borderRadius 收敛为 4 档语义令牌。

映射（见 docs/frontend-v2-visual-hybrid-plan.md P0）：
  >= 14（页面外壳 / 大卡片）  → var(--radius-structure)  直角
  4 ~ 12（次级内容区 / 控件）→ var(--radius-control)     4px
  50 / 50%（头像）           → var(--radius-avatar)      不动
  999 / 999px（胶囊）        → var(--radius-pill)        不动

数字写法（borderRadius: 8）统一改为字符串写法（borderRadius: 'var(--radius-control)'），
否则 React 会输出 "8px" 而无法引用变量。

跳过 AI Office 3D（src/components/office）。

用法：
  python3 normalize_radius.py           # dry-run
  python3 normalize_radius.py --apply   # 写入
"""
import os
import re
import sys
import collections

ROOT = os.path.abspath(
    os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', '..', 'frontend-v2', 'src')
)
SKIP_DIRS = {'office'}
APPLY = '--apply' in sys.argv

RADIUS_PROPS = (
    'borderRadius',
    'borderTopLeftRadius', 'borderTopRightRadius',
    'borderBottomLeftRadius', 'borderBottomRightRadius',
    'borderStartStartRadius', 'borderStartEndRadius',
    'borderEndStartRadius', 'borderEndEndRadius',
)

# 匹配 prop: 数字  /  prop: '值'  （两种写法都要处理）
DECL_RE = re.compile(
    r"(?P<prop>" + '|'.join(RADIUS_PROPS) + r")"
    r"\s*:\s*(?:(?P<num>\d+(?:\.\d+)?)(?P<unit>px|%)?|'(?P<str>[^']*)')"
)


def token_for(value: float, unit: str) -> str | None:
    """按数值大小归类到四档令牌；返回 None 表示无法判断（原样保留）"""
    if unit == '%':
        if value >= 50:
            return 'var(--radius-avatar)'
        return None
    if value >= 999:
        return 'var(--radius-pill)'
    if value >= 50:
        return 'var(--radius-avatar)'
    if value >= 14:
        return 'var(--radius-structure)'
    if value > 0:
        return 'var(--radius-control)'
    return 'var(--radius-structure)'   # 0


def main():
    stats = collections.Counter()
    changed = []
    skipped = []

    for dp, _dn, fnames in os.walk(ROOT):
        for fname in fnames:
            if not fname.endswith('.tsx'):
                continue
            path = os.path.join(dp, fname)
            rel = os.path.relpath(path, ROOT).replace('\\', '/')
            if rel.split('/')[0] in SKIP_DIRS:
                continue

            with open(path, encoding='utf-8') as f:
                src = f.read()
            out = src

            for m in reversed(list(DECL_RE.finditer(src))):
                prop = m.group('prop')
                if m.group('str') is not None:
                    raw = m.group('str').strip()
                    sm = re.fullmatch(r'(\d+(?:\.\d+)?)(px|%)?', raw)
                    if not sm:
                        skipped.append((rel, prop, raw))
                        continue
                    value, unit = float(sm.group(1)), sm.group(2) or ''
                else:
                    value = float(m.group('num'))
                    unit = m.group('unit') or ''

                tok = token_for(value, unit)
                if tok is None:
                    skipped.append((rel, prop, f'{value}{unit}'))
                    continue

                new = f"{prop}: '{tok}'"
                out = out[:m.start()] + new + out[m.end():]
                stats[tok] += 1

            if out != src:
                changed.append(rel)
                if APPLY:
                    with open(path, 'w', encoding='utf-8', newline='') as f:
                        f.write(out)

    print(f'=== {"已写入" if APPLY else "DRY-RUN（未写入）"} ===')
    print(f'改动文件数：{len(changed)}')
    for k, v in stats.most_common():
        print(f'  {k:26s} {v}')
    print(f'  合计 {sum(stats.values())}')
    if skipped:
        print(f'\n=== 无法判断（保留原样）：{len(skipped)} 处 ===')
        agg = collections.Counter((p, v) for _, p, v in skipped)
        for (prop, val), n in agg.most_common(15):
            print(f'  {n:3d}  {prop}: {val}')


if __name__ == '__main__':
    main()
