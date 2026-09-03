#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
把 frontend-v2 内联样式里的硬编码色值替换为设计令牌 var(--x)。

安全策略（宁可漏替换，也不错误替换）：
  1. 只处理 `prop: '...'` 形式；值为单色时整值替换，border 简写只替换颜色部分。
  2. 渐变 / 多色值一律跳过并报告，交人工处理。
  3. 按 CSS 属性语义分表映射：background / border / color / backdropFilter。
  4. 状态色（success/warning/error/...）全局优先；灰阶只在文字语义下替换。
  5. 跳过 AI Office 3D（src/components/office，独立视觉体系，方案明确不涉及）。

用法：
  python3 tokenize_styles.py            # dry-run，只报告
  python3 tokenize_styles.py --apply    # 真正写入
"""
import os
import re
import sys
import collections

# 本脚本位于 <repo>/deploy/.tmp/，向上两级即仓库根
ROOT = os.path.abspath(
    os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', '..', 'frontend-v2', 'src')
)
SKIP_DIRS = {'office'}          # AI Office 3D：独立视觉体系
APPLY = '--apply' in sys.argv

# ---------- 映射表 ----------

# 状态/品牌色：任何属性下都安全，优先匹配
STATE_MAP = {
    '#22c55e': 'var(--success)',
    '#10b981': 'var(--success)',
    '#34d399': 'var(--success)',
    '#6ee7b7': 'var(--success)',
    '#27a644': 'var(--success)',
    '#6dc67f': 'var(--success)',
    '#f59e0b': 'var(--warning)',
    '#fb923c': 'var(--warning)',
    '#fbbf24': 'var(--warning)',
    '#fcd34d': 'var(--warning)',
    '#ef4444': 'var(--error)',
    '#f87171': 'var(--error)',
    '#f43f5e': 'var(--error)',
    '#fda4af': 'var(--error)',
    '#3b82f6': 'var(--info)',
    '#60a5fa': 'var(--info)',
    '#93c5fd': 'var(--info)',
    '#22d3ee': 'var(--cyan)',
    '#67e8f9': 'var(--cyan)',
    '#6d5ff5': 'var(--signal)',
    '#a5b4fc': 'var(--signal-hover)',
    '#7dd3fc': 'var(--info)',
    '#0ea5e9': 'var(--info)',
    '#4ade80': 'var(--success)',
    '#8b5cf6': 'var(--signal)',
    '#a855f7': 'var(--signal)',
    '#9e4cff': 'var(--signal)',
    '#8b7ff8': 'var(--signal-hover)',
    '#a99cff': 'var(--signal-hover)',
    '#c084fc': 'var(--signal-hover)',
    '#c4b5fd': 'var(--signal-hover)',
    '#e879f9': 'var(--signal-hover)',
}

# 灰阶：只作文字/次要元素色，不放 STATE_MAP（避免误替换作背景的用法）
GRAY_MAP = {
    '#f4f4f8': 'var(--text-primary)',
    '#e4e4e7': 'var(--text-secondary)',
    '#d4d4d8': 'var(--text-secondary)',
    '#a1a1aa': 'var(--text-tertiary)',
    '#9ca3af': 'var(--text-tertiary)',
    '#94a3b8': 'var(--text-tertiary)',
    '#8b8f9e': 'var(--text-tertiary)',
    '#71717a': 'var(--text-quaternary)',
    '#6b7280': 'var(--text-quaternary)',
    '#64748b': 'var(--text-quaternary)',
}

# background / backgroundColor
BG_MAP = {
    'rgba(255,255,255,0.02)': 'var(--surface-sunken)',
    'rgba(255,255,255,0.03)': 'var(--surface)',
    'rgba(255,255,255,0.04)': 'var(--surface)',
    'rgba(255,255,255,0.05)': 'var(--surface)',
    'rgba(255,255,255,0.06)': 'var(--surface-raised)',
    'rgba(255,255,255,0.07)': 'var(--surface-raised)',
    'rgba(255,255,255,0.08)': 'var(--surface-raised)',
    'rgba(255,255,255,0.12)': 'var(--surface-raised)',
    '#0a0a12': 'var(--canvas)',
    '#0a0a14': 'var(--canvas)',
    '#0b0b10': 'var(--canvas)',
    '#12121d': 'var(--canvas-elevated)',
    '#171722': 'var(--canvas-elevated)',
    '#0f0f18': 'var(--canvas-elevated)',
    '#1b1b26': 'var(--canvas-elevated)',
    '#94a3b8': 'var(--surface-raised)',
    '#6b7280': 'var(--surface-raised)',
    '#3f4450': 'var(--surface-raised)',
    'rgba(255,255,255,0.4)': 'var(--surface-raised)',
    'rgba(255,255,255,0.2)': 'var(--surface-raised)',
    'rgba(255,255,255,0.1)': 'var(--surface-raised)',
}

# border / borderColor / outline
LINE_MAP = {
    'rgba(255,255,255,0.04)': 'var(--line)',
    'rgba(255,255,255,0.05)': 'var(--line)',
    'rgba(255,255,255,0.06)': 'var(--line)',
    'rgba(255,255,255,0.07)': 'var(--line)',
    'rgba(255,255,255,0.08)': 'var(--line)',
    'rgba(255,255,255,0.09)': 'var(--line)',
    'rgba(255,255,255,0.1)': 'var(--line-strong)',
    'rgba(255,255,255,0.12)': 'var(--line-strong)',
    'rgba(255,255,255,0.14)': 'var(--line-strong)',
    'rgba(255,255,255,0.15)': 'var(--line-strong)',
    'rgba(255,255,255,0.16)': 'var(--line-strong)',
    'rgba(255,255,255,0.2)': 'var(--line-strong)',
    'rgba(255,255,255,0.3)': 'var(--line-strong)',
    '#232330': 'var(--line)',
    '#232331': 'var(--line)',
    '#1b1b26': 'var(--line)',
    '#3f4450': 'var(--line-strong)',
    '#6b7280': 'var(--line-strong)',
}

# color / fill
TEXT_MAP = {
    'rgba(255,255,255,0.95)': 'var(--text-primary)',
    'rgba(255,255,255,0.9)': 'var(--text-primary)',
    'rgba(255,255,255,0.85)': 'var(--text-primary)',
    'rgba(255,255,255,0.8)': 'var(--text-primary)',
    'rgba(255,255,255,0.75)': 'var(--text-secondary)',
    '#0a0a12': 'var(--text-inverse)',
    'rgba(255,255,255,0.7)': 'var(--text-secondary)',
    'rgba(255,255,255,0.65)': 'var(--text-secondary)',
    'rgba(255,255,255,0.6)': 'var(--text-secondary)',
    'rgba(255,255,255,0.55)': 'var(--text-tertiary)',
    'rgba(255,255,255,0.5)': 'var(--text-tertiary)',
    'rgba(255,255,255,0.45)': 'var(--text-tertiary)',
    'rgba(255,255,255,0.4)': 'var(--text-quaternary)',
    'rgba(255,255,255,0.35)': 'var(--text-quaternary)',
    'rgba(255,255,255,0.32)': 'var(--text-quaternary)',
    'rgba(255,255,255,0.3)': 'var(--text-quaternary)',
    'rgba(255,255,255,0.25)': 'var(--text-quaternary)',
    'rgba(255,255,255,0.2)': 'var(--text-quaternary)',
    '#c8ccd8': 'var(--text-secondary)',
    '#e8e8f4': 'var(--text-primary)',
    '#9aa3b2': 'var(--text-tertiary)',
    '#a78bfa': 'var(--signal)',
    '#c4b9ff': 'var(--signal-hover)',
    '#fecaca': 'var(--error)',
    '#a7f3d0': 'var(--success)',
    'rgba(255,255,255,0.88)': 'var(--text-primary)',
    'rgba(255,255,255,0.72)': 'var(--text-secondary)',
    'rgba(255,255,255,0.38)': 'var(--text-quaternary)',
    'rgba(255,255,255,0.22)': 'var(--text-quaternary)',
    'rgba(255,255,255,0.42)': 'var(--text-tertiary)',
    'rgba(255,255,255,0.62)': 'var(--text-secondary)',
    'rgba(255,255,255,0.78)': 'var(--text-secondary)',
    '#e8e8f0': 'var(--text-primary)',
}

BG_PROPS = {'background', 'backgroundColor', 'bg'}
BORDER_PROPS = {
    'border', 'borderTop', 'borderBottom', 'borderLeft', 'borderRight',
    'borderColor', 'borderTopColor', 'borderBottomColor',
    'borderLeftColor', 'borderRightColor', 'outline',
}
TEXT_PROPS = {'color', 'fill', 'text'}

COLOR_RE = re.compile(r'rgba\(\s*255\s*,\s*255\s*,\s*255\s*,\s*0?\.\d+\s*\)|#[0-9a-fA-F]{6}\b')
# 匹配 prop: 'value'（值内不含单引号）
DECL_RE = re.compile(r"(?P<prop>[A-Za-z][A-Za-z0-9]*)\s*:\s*'(?P<val>[^']*)'")
BLUR_RE = re.compile(r'^blur\([^)]*\)(\s+saturate\([^)]*\))?$')


def norm(c: str) -> str:
    """
    统一颜色写法：去空格、小写、去小数尾随零。

    注意：只能 rstrip 去掉「末尾的零」，不能用 int() 重新格式化——
    int('03') = 3 会把 0.03（3% 不透明）变成 0.3（30%），是完全不同的颜色。
    """
    c = re.sub(r'\s+', '', c).lower()
    m = re.fullmatch(r'rgba\(255,255,255,0\.(\d+)\)', c)
    if m:
        digits = m.group(1).rstrip('0') or '0'
        return f'rgba(255,255,255,0.{digits})'
    return c


def map_color(prop: str, raw: str):
    """返回 (新值, 命中表名) 或 (None, None)"""
    c = norm(raw)
    if c in STATE_MAP:
        return STATE_MAP[c], 'state'
    if prop in BG_PROPS:
        return BG_MAP.get(c), 'bg'
    if prop in BORDER_PROPS:
        return LINE_MAP.get(c), 'line'
    if prop in TEXT_PROPS:
        if c in TEXT_MAP:
            return TEXT_MAP[c], 'text'
        return GRAY_MAP.get(c), 'text'
    # 未知属性（如 statusColors 里的 dot / offline / asking）：
    # 只允许状态色、灰阶与文字层级，不猜表面/线条语义
    return GRAY_MAP.get(c) or TEXT_MAP.get(c), 'text'


def process(src: str, rel: str, stats, skipped):
    out = src
    # 从后往前替换，避免 offset 失效
    for m in reversed(list(DECL_RE.finditer(src))):
        prop, val = m.group('prop'), m.group('val')
        full = val.strip()

        # backdropFilter：整值收敛到 --glass-blur
        if prop in ('backdropFilter', 'WebkitBackdropFilter'):
            if BLUR_RE.match(full):
                out = out[:m.start('val')] + 'var(--glass-blur)' + out[m.end('val'):]
                stats['glass'] += 1
            else:
                skipped.append((rel, prop, val))
            continue

        # A 类：整值就是单个颜色
        if COLOR_RE.fullmatch(full):
            new, table = map_color(prop, full)
            if new:
                out = out[:m.start('val')] + new + out[m.end('val'):]
                stats[table] += 1
            else:
                skipped.append((rel, prop, val))
            continue

        # B 类：border 简写 '1px solid X' —— 只替换颜色部分
        bm = re.fullmatch(r'(?P<pre>[\d.]+\w*\s+\w+\s+)(?P<col>.+)', full)
        if bm and COLOR_RE.fullmatch(bm.group('col').strip()) and prop in BORDER_PROPS:
            new, table = map_color(prop, bm.group('col').strip())
            if new:
                out = out[:m.start('val')] + bm.group('pre') + new + out[m.end('val'):]
                stats[table] += 1
            else:
                skipped.append((rel, prop, val))
            continue

        # C 类：渐变 / 多色 / 其他 —— 跳过
        if 'gradient' in full or COLOR_RE.search(full):
            skipped.append((rel, prop, val))
    return out


def main():
    stats = collections.Counter()
    skipped = []
    changed = []

    for dp, _dn, fnames in os.walk(ROOT):
        for fname in fnames:
            if not fname.endswith('.tsx'):
                continue
            path = os.path.join(dp, fname)
            rel = os.path.relpath(path, ROOT).replace('\\', '/')
            if rel.split('/')[0] in SKIP_DIRS:
                continue
            with open(path, encoding='utf-8') as f:
                before = f.read()
            after = process(before, rel, stats, skipped)
            if after != before:
                changed.append(rel)
                if APPLY:
                    with open(path, 'w', encoding='utf-8', newline='') as f:
                        f.write(after)

    print(f'=== {"已写入" if APPLY else "DRY-RUN（未写入）"} ===')
    print(f'改動文件数：{len(changed)}')
    for k in ('state', 'bg', 'line', 'text', 'glass'):
        print(f'  {k:8s} {stats[k]}')
    print(f'  合计     {sum(stats.values())}')

    print(f'\n=== 跳过（需人工处理）：{len(skipped)} 处 ===')
    agg = collections.Counter((p, v) for _, p, v in skipped)
    for (prop, val), n in agg.most_common(30):
        print(f'  {n:3d}  {prop}: {val[:66]}')


if __name__ == '__main__':
    main()
